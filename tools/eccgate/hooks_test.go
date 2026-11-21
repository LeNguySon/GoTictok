package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// payload builds the JSON Claude Code sends to a hook.
func payload(t *testing.T, command, filePath string, stopActive bool) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"stop_hook_active": stopActive,
		"tool_input":       map[string]string{"command": command, "file_path": filePath},
	})
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return string(data)
}

// hookDecision is what a hook wrote to stdout. PreToolUse hooks answer in
// hookSpecificOutput; PostToolUse and Stop hooks answer with decision/reason.
type hookDecision struct {
	Decision           string `json:"decision"`
	Reason             string `json:"reason"`
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

// readDecision parses hook stdout; silence means "no objection".
func readDecision(t *testing.T, stdout string) hookDecision {
	t.Helper()
	var d hookDecision
	if strings.TrimSpace(stdout) == "" {
		return d
	}
	if err := json.Unmarshal([]byte(stdout), &d); err != nil {
		t.Fatalf("hook stdout is not JSON: %v\n%s", err, stdout)
	}
	return d
}

// runHook runs a hook that must never fail as a process: Claude Code only
// honours a decision that arrives with exit status 0.
func runHook(t *testing.T, tg *testGate, name string) hookDecision {
	t.Helper()
	if code := tg.hook(context.Background(), name); code != exitOK {
		t.Fatalf("hook(%q) = %d, want %d (stderr %q)", name, code, exitOK, tg.stderr)
	}
	return readDecision(t, tg.stdout.String())
}

func TestHookRejectsBadInvocations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		hook  string
		stdin string
	}{
		{name: "unknown hook", hook: "nope", stdin: "{}"},
		{name: "invalid JSON", hook: "pre-bash", stdin: "{not json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, tt.stdin)
			if code := tg.hook(context.Background(), tt.hook); code != exitFailed {
				t.Errorf("hook(%q) = %d, want %d", tt.hook, code, exitFailed)
			}
		})
	}
}

func TestHookAcceptsEmptyInput(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	if d := runHook(t, tg, "pre-bash"); d != (hookDecision{}) {
		t.Errorf("hook() decided %+v on empty input, want no objection", d)
	}
}

func TestHookPreBash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		command      string
		wantDecision string
		wantReason   string
	}{
		{name: "ordinary command passes", command: "go test ./..."},
		{name: "hook bypass is denied", command: `git commit --no-verify -m "x"`, wantDecision: "deny", wantReason: "--no-verify skips the git hooks"},
		{name: "hooks path change is denied", command: `git config core.hooksPath /dev/null`, wantDecision: "deny", wantReason: "core.hooksPath"},
		{name: "rewriting the gate needs a human", command: "rm .githooks/pre-commit", wantDecision: "ask", wantReason: ".githooks/pre-commit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, payload(t, tt.command, "", false))
			out := runHook(t, tg, "pre-bash").HookSpecificOutput

			if out.PermissionDecision != tt.wantDecision {
				t.Errorf("permission decision = %q, want %q", out.PermissionDecision, tt.wantDecision)
			}
			if !strings.Contains(out.PermissionDecisionReason, tt.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", out.PermissionDecisionReason, tt.wantReason)
			}
			if tt.wantDecision != "" && out.HookEventName != "PreToolUse" {
				t.Errorf("hook event = %q, want PreToolUse", out.HookEventName)
			}
		})
	}
}

func TestHookPreEdit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		file         string
		wantDecision string
	}{
		{name: "application code is free to edit", file: "internal/app/app.go"},
		{name: "lint config needs a human", file: `C:\repo\.golangci.yml`, wantDecision: "ask"},
		{name: "the gate itself needs a human", file: "tools/eccgate/coverage.go", wantDecision: "ask"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, payload(t, "", tt.file, false))
			out := runHook(t, tg, "pre-edit").HookSpecificOutput

			if out.PermissionDecision != tt.wantDecision {
				t.Errorf("permission decision = %q, want %q", out.PermissionDecision, tt.wantDecision)
			}
		})
	}
}

func TestHookPostEdit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		file      string // relative to the repository root
		create    bool
		failOn    string
		wantBlock bool
		wantCalls int
	}{
		{name: "go file is formatted and vetted", file: "internal/app/app.go", create: true, wantCalls: 2},
		{name: "gofmt failure goes back to the agent", file: "internal/app/app.go", create: true, failOn: "gofmt", wantBlock: true, wantCalls: 1},
		{name: "vet failure goes back to the agent", file: "internal/app/app.go", create: true, failOn: "go vet", wantBlock: true, wantCalls: 2},
		{name: "non-go file is ignored", file: "README.md", create: true},
		{name: "deleted go file is ignored", file: "internal/app/gone.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			file := filepath.Join(tg.root, filepath.FromSlash(tt.file))
			if tt.create {
				writeFile(t, file, "package app\n")
			}
			if tt.failOn != "" {
				tg.failOn(tt.failOn, "problem found")
			}
			tg.stdin = strings.NewReader(payload(t, "", file, false))

			d := runHook(t, tg, "post-edit")

			if got := d.Decision == "block"; got != tt.wantBlock {
				t.Errorf("blocked = %v, want %v (decision %+v)", got, tt.wantBlock, d)
			}
			if tt.wantBlock && !strings.Contains(d.Reason, "problem found") {
				t.Errorf("reason = %q, want it to carry the tool output", d.Reason)
			}
			if len(tg.fake.calls) != tt.wantCalls {
				t.Errorf("ran %v, want %d command(s)", tg.fake.calls, tt.wantCalls)
			}
		})
	}
}

func TestHookPostEditIgnoresFilesOutsideTheRepository(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	outside := filepath.Join(t.TempDir(), "scratch.go")
	writeFile(t, outside, "package scratch\n")
	tg.stdin = strings.NewReader(payload(t, "", outside, false))

	if d := runHook(t, tg, "post-edit"); d.Decision != "" {
		t.Errorf("hook() decided %+v for a file outside the repository", d)
	}
	if len(tg.fake.calls) != 0 {
		t.Errorf("ran %v on a file outside the repository", tg.fake.calls)
	}
}

func TestHookStop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     string
		stopActive bool
		breakBuild bool
		wantBlock  bool
		wantVerify bool
	}{
		{name: "clean tree stops freely", status: ""},
		{name: "only docs changed", status: " M README.md\n?? docs/notes.md\n"},
		{name: "green go changes may stop", status: " M internal/app/app.go\n", wantVerify: true},
		{name: "red go changes are sent back", status: "?? internal/feed/feed.go\n", breakBuild: true, wantBlock: true, wantVerify: true},
		{name: "module file change triggers the gate", status: " M go.mod\n", breakBuild: true, wantBlock: true, wantVerify: true},
		{name: "second stop attempt is not blocked again", status: " M internal/app/app.go\n", stopActive: true, breakBuild: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.healthyRepo(t, coveredProfile)
			tg.succeedWith("git status", tt.status)
			if tt.breakBuild {
				tg.failOn("go build", "undefined: Feed")
			}
			tg.stdin = strings.NewReader(payload(t, "", "", tt.stopActive))

			d := runHook(t, tg, "stop")

			if got := d.Decision == "block"; got != tt.wantBlock {
				t.Errorf("blocked = %v, want %v (decision %+v)", got, tt.wantBlock, d)
			}
			if got := tg.fake.called("go build"); got != tt.wantVerify {
				t.Errorf("gate ran = %v, want %v", got, tt.wantVerify)
			}
			if tt.wantBlock && !strings.Contains(d.Reason, "undefined: Feed") {
				t.Errorf("reason does not carry the gate report: %q", d.Reason)
			}
		})
	}
}

func TestHookStopAllowsStoppingOutsideGit(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.failOn("git status", "not a git repository")

	if d := runHook(t, tg, "stop"); d.Decision != "" {
		t.Errorf("hook() decided %+v outside a git repository", d)
	}
}

func TestHookSessionStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ready       bool
		failGit     bool
		wantWarning string
	}{
		{name: "ready clone gets the reminder only", ready: true},
		{name: "missing tools are flagged", wantWarning: "not fully set up"},
		{name: "hook activation failure is flagged", ready: true, failGit: true, wantWarning: "git hooks are not active"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			if tt.ready {
				tg.healthyRepo(t, coveredProfile)
			}
			if tt.failGit {
				tg.failOn("git config core.hooksPath", "permission denied")
			}

			if code := tg.hook(context.Background(), "session-start"); code != exitOK {
				t.Fatalf("hook() = %d, want %d", code, exitOK)
			}
			out := tg.stdout.String()
			if !strings.Contains(out, "AGENTS.md is binding") {
				t.Errorf("stdout lacks the contract reminder: %q", out)
			}
			if got := strings.Contains(out, "WARNING"); got != (tt.wantWarning != "") {
				t.Errorf("warning shown = %v, want %v: %q", got, tt.wantWarning != "", out)
			}
			if !strings.Contains(out, tt.wantWarning) {
				t.Errorf("stdout = %q, want it to contain %q", out, tt.wantWarning)
			}
			if !tg.fake.called("git config core.hooksPath " + hooksDir) {
				t.Errorf("git hooks were not activated: %v", tg.fake.calls)
			}
		})
	}
}
