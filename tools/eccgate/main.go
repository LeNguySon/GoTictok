// Command eccgate is the enforcement gate for the ECC workflow described in
// AGENTS.md.
//
// Instructions alone cannot make an AI agent comply, so every rule that can
// be checked mechanically is checked here. Git hooks, CI and agent hooks all
// call this one program, which means the rules are applied the same way no
// matter which agent, or which human, wrote the code.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
)

const usage = `eccgate enforces the ECC workflow (see AGENTS.md).

Usage: go run ./tools/eccgate <command>

  setup [--tools-only]   activate the git hooks and install the pinned tools
  doctor                 report whether this clone is ready to enforce the contract
  verify [--quick]       run the gate (full by default; --quick skips network checks)
  pre-commit             git hook: quick gate
  pre-push               git hook: full gate
  commit-msg <file>      git hook: check the commit message format
  commits [<rev-range>]  check commit subjects (default: the last commit)
  hook <name>            agent hook: session-start, pre-bash, pre-edit, post-edit, stop
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := start(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// start builds the real gate for the repository around the working directory.
func start(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	errOut := &printer{w: stderr}
	root, err := findRoot(".")
	if err != nil {
		errOut.printf("eccgate: %v\n", err)
		return exitFailed
	}
	g := &gate{root: root, run: execRunner{}, stdin: stdin, out: &printer{w: stdout}, errOut: errOut}
	return run(ctx, args, g)
}

// run executes one command and returns the process exit code.
func run(ctx context.Context, args []string, g *gate) int {
	if len(args) == 0 {
		g.errOut.printf("%s", usage)
		return exitFailed
	}
	code := g.dispatch(ctx, args[0], args[1:])
	if code == exitOK && (g.out.err != nil || g.errOut.err != nil) {
		return exitFailed
	}
	return code
}

func (g *gate) dispatch(ctx context.Context, name string, args []string) int {
	switch name {
	case "verify":
		if slices.Contains(args, "--quick") {
			return g.cmdVerify(ctx, quick)
		}
		return g.cmdVerify(ctx, full)
	case "pre-commit":
		return g.cmdVerify(ctx, quick)
	case "pre-push":
		return g.cmdVerify(ctx, full)
	case "commit-msg":
		return g.cmdCommitMsg(args)
	case "commits":
		return g.cmdCommits(ctx, args)
	case "hook":
		return g.cmdHook(ctx, args)
	case "setup":
		return g.cmdSetup(ctx, slices.Contains(args, "--tools-only"))
	case "doctor":
		return g.cmdDoctor(ctx)
	}
	g.errOut.printf("eccgate: unknown command %q\n\n%s", name, usage)
	return exitFailed
}

func (g *gate) cmdVerify(ctx context.Context, p profile) int {
	if g.verify(ctx, p, g.out) {
		return exitOK
	}
	return exitFailed
}

func (g *gate) cmdCommitMsg(args []string) int {
	if len(args) != 1 {
		g.errOut.printf("usage: eccgate commit-msg <file>\n")
		return exitFailed
	}
	//nolint:gosec // G304: git passes the path of its own commit message file.
	data, err := os.ReadFile(args[0])
	if err != nil {
		g.errOut.printf("eccgate: read commit message: %v\n", err)
		return exitFailed
	}
	if err := validateCommitMessage(string(data)); err != nil {
		g.errOut.printf("ECC gate: commit rejected: %v\n", err)
		return exitFailed
	}
	return exitOK
}

// cmdCommits checks the subjects of commits that already exist, which is how
// CI catches a commit made with the local hooks skipped.
func (g *gate) cmdCommits(ctx context.Context, args []string) int {
	logArgs := []string{"log", "--format=%s", "--no-merges"}
	if len(args) > 0 {
		logArgs = append(logArgs, "--end-of-options", args[0])
	} else {
		logArgs = append(logArgs, "-n", "1")
	}
	out, err := g.run.Run(ctx, command{dir: g.root, name: "git", args: logArgs})
	if err != nil {
		g.errOut.printf("eccgate: read commit history: %v\n%s\n", err, tail(out))
		return exitFailed
	}

	rejected := 0
	for _, subject := range strings.Split(strings.TrimSpace(out), "\n") {
		if subject == "" {
			continue
		}
		if err := validateCommitMessage(subject); err != nil {
			rejected++
			g.errOut.printf("ECC gate: %v\n", err)
		}
	}
	if rejected > 0 {
		return exitFailed
	}
	g.out.printf("commit subjects follow the ECC format\n")
	return exitOK
}

func (g *gate) cmdHook(ctx context.Context, args []string) int {
	if len(args) != 1 {
		g.errOut.printf("usage: eccgate hook <name>\n")
		return exitFailed
	}
	return g.hook(ctx, args[0])
}

func (g *gate) cmdSetup(ctx context.Context, toolsOnly bool) int {
	if err := g.setup(ctx, toolsOnly); err != nil {
		g.errOut.printf("eccgate: setup failed: %v\n", err)
		return exitFailed
	}
	return exitOK
}

func (g *gate) cmdDoctor(ctx context.Context) int {
	g.out.printf("ECC gate status\n")
	if !g.doctor(ctx, g.out) {
		return exitFailed
	}
	return exitOK
}
