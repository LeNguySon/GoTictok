package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Agent hooks answer with a JSON decision on stdout and exit status 0.
//
// Exit status 2 would also block an action, but the hooks are started with
// `go run`, which reports every failing program as status 1. Claude Code
// reads status 1 as "the hook crashed" and lets the action through, so an
// exit-code protocol would fail open. The JSON protocol cannot.

// hookInput is the part of the Claude Code hook payload the gate reads.
type hookInput struct {
	StopHookActive bool `json:"stop_hook_active"`
	ToolInput      struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

const sessionBanner = `ECC workflow is enforced in this repository. AGENTS.md is binding for every agent.
- Explore with the knowledge graph first: graphify query "<question>"
- Work test-first (RED, GREEN, REFACTOR); coverage must stay at or above 80%.
- Before you finish or commit: go run ./tools/eccgate verify
- Never bypass the gate (--no-verify, core.hooksPath) and never weaken its config.
`

// hook runs the agent hook called name with the JSON payload on stdin.
func (g *gate) hook(ctx context.Context, name string) int {
	handlers := map[string]func(context.Context, hookInput) int{
		"session-start": g.hookSessionStart,
		"pre-bash":      g.hookPreBash,
		"pre-edit":      g.hookPreEdit,
		"post-edit":     g.hookPostEdit,
		"stop":          g.hookStop,
	}
	handler, ok := handlers[name]
	if !ok {
		g.errOut.printf("eccgate: unknown hook %q\n", name)
		return exitFailed
	}

	var in hookInput
	data, err := io.ReadAll(g.stdin)
	if err != nil {
		g.errOut.printf("eccgate: read hook input: %v\n", err)
		return exitFailed
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &in); err != nil {
			g.errOut.printf("eccgate: parse hook input: %v\n", err)
			return exitFailed
		}
	}
	return handler(ctx, in)
}

// emit writes a hook decision as one line of JSON.
func (g *gate) emit(decision any) int {
	data, err := json.Marshal(decision)
	if err != nil {
		g.errOut.printf("eccgate: encode hook decision: %v\n", err)
		return exitFailed
	}
	g.out.printf("%s\n", data)
	return exitOK
}

// preToolDecision answers a PreToolUse hook. "deny" cancels the tool call and
// gives the reason to the agent; "ask" turns the call into a prompt for a human.
func (g *gate) preToolDecision(decision, reason string) int {
	return g.emit(map[string]any{
		"hookSpecificOutput": map[string]string{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       decision,
			"permissionDecisionReason": reason,
		},
	})
}

// block answers a PostToolUse or Stop hook: the agent is sent back to work
// with reason as its instructions.
func (g *gate) block(reason string) int {
	return g.emit(map[string]string{"decision": "block", "reason": reason})
}

// askHuman requires a person to approve a change to the gate's own files.
func (g *gate) askHuman(target string) int {
	return g.preToolDecision("ask", fmt.Sprintf("%s is part of the ECC enforcement gate. "+
		"Agents must fix the code, not weaken the gate, so a human has to approve this change.", target))
}

// hookSessionStart makes sure the git hooks are wired up (a fresh clone has
// none) and reminds the agent of the contract.
func (g *gate) hookSessionStart(ctx context.Context, _ hookInput) int {
	if err := g.ensureHooksPath(ctx); err != nil {
		g.out.printf("WARNING: git hooks are not active: %v\n", err)
	}
	g.out.printf("%s", sessionBanner)
	if !g.doctor(ctx, &printer{w: io.Discard}) {
		g.out.printf("WARNING: the gate is not fully set up. Run: go run ./tools/eccgate setup\n")
	}
	return exitOK
}

// hookPreBash denies commands that skip the git hooks and asks a human
// before a command rewrites the gate's own files.
func (g *gate) hookPreBash(_ context.Context, in hookInput) int {
	cmdline := in.ToolInput.Command
	if reason := bypassReason(cmdline); reason != "" {
		return g.preToolDecision("deny", "ECC gate: "+reason+". "+
			"The gate may not be bypassed (AGENTS.md, \"Forbidden\"). Fix what it reports instead.")
	}
	if target := mutatedProtectedPath(cmdline); target != "" {
		return g.askHuman(target)
	}
	return exitOK
}

// hookPreEdit asks a human before an agent edits the gate's own files.
func (g *gate) hookPreEdit(_ context.Context, in hookInput) int {
	if isProtected(in.ToolInput.FilePath) {
		return g.askHuman(in.ToolInput.FilePath)
	}
	return exitOK
}

// hookPostEdit formats a Go file the agent has just written and vets its
// package, feeding any problem straight back to the agent.
func (g *gate) hookPostEdit(ctx context.Context, in hookInput) int {
	file := in.ToolInput.FilePath
	if !strings.HasSuffix(file, ".go") || !g.contains(file) {
		return exitOK
	}
	if _, err := os.Stat(file); err != nil {
		return exitOK // the edit removed the file
	}

	if out, err := g.run.Run(ctx, command{dir: g.root, name: "gofmt", args: []string{"-w", file}}); err != nil {
		return g.block(fmt.Sprintf("ECC gate: gofmt rejected %s:\n%s", file, tail(out)))
	}
	dir := filepath.Dir(file)
	if out, err := g.run.Run(ctx, command{dir: dir, name: "go", args: []string{"vet", "."}}); err != nil {
		return g.block(fmt.Sprintf("ECC gate: go vet reports problems in %s "+
			"(an undefined symbol is expected only during the RED step of TDD):\n%s", dir, tail(out)))
	}
	return exitOK
}

// contains reports whether file lies inside the repository.
func (g *gate) contains(file string) bool {
	abs, err := filepath.Abs(file)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(g.root, abs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// hookStop refuses to let an agent finish while its Go changes fail the gate.
// It blocks once per stop attempt: when the agent stops again after being
// sent back, the commit, push and CI gates remain the backstop.
func (g *gate) hookStop(ctx context.Context, in hookInput) int {
	if in.StopHookActive || !g.hasUncommittedGoChanges(ctx) {
		return exitOK
	}

	var report bytes.Buffer
	if g.verify(ctx, quick, &printer{w: &report}) {
		return exitOK
	}
	return g.block(report.String() + "\nECC gate: the work is not finished while the gate is red. " +
		"Fix the failures above, then re-run: go run ./tools/eccgate verify --quick")
}

// hasUncommittedGoChanges reports whether the working tree has modified or
// new Go sources or module files.
func (g *gate) hasUncommittedGoChanges(ctx context.Context) bool {
	out, err := g.run.Run(ctx, command{dir: g.root, name: "git", args: []string{"status", "--porcelain", "--untracked-files=all"}})
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		file := strings.Trim(strings.TrimSpace(line), `"`)
		if strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "go.mod") || strings.HasSuffix(file, "go.sum") {
			return true
		}
	}
	return false
}
