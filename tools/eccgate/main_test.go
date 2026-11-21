package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestRunDispatchesCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantCalled string
		wantSkip   string
	}{
		{name: "verify is the full gate", args: []string{"verify"}, wantCalled: "govulncheck"},
		{name: "verify --quick skips network checks", args: []string{"verify", "--quick"}, wantCalled: "go test -race", wantSkip: "govulncheck"},
		{name: "pre-commit is the quick gate", args: []string{"pre-commit"}, wantCalled: "go test -race", wantSkip: "govulncheck"},
		{name: "pre-push is the full gate", args: []string{"pre-push"}, wantCalled: "govulncheck"},
		{name: "doctor reports status", args: []string{"doctor"}, wantCalled: "git config --get core.hooksPath"},
		{name: "setup activates hooks", args: []string{"setup"}, wantCalled: "git config core.hooksPath"},
		{name: "setup --tools-only skips git", args: []string{"setup", "--tools-only"}, wantSkip: "git config"},
		{name: "hook runs the named hook", args: []string{"hook", "pre-bash"}},
		{name: "no command prints usage", args: nil, wantCode: exitFailed},
		{name: "unknown command prints usage", args: []string{"frobnicate"}, wantCode: exitFailed},
		{name: "hook needs a name", args: []string{"hook"}, wantCode: exitFailed},
		{name: "commit-msg needs a file", args: []string{"commit-msg"}, wantCode: exitFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.healthyRepo(t, coveredProfile)

			if code := run(context.Background(), tt.args, tg.gate); code != tt.wantCode {
				t.Errorf("run(%v) = %d, want %d\nstdout: %s\nstderr: %s", tt.args, code, tt.wantCode, tg.stdout, tg.stderr)
			}
			if tt.wantCalled != "" && !tg.fake.called(tt.wantCalled) {
				t.Errorf("command %q was not run; ran %v", tt.wantCalled, tg.fake.calls)
			}
			if tt.wantSkip != "" && tg.fake.called(tt.wantSkip) {
				t.Errorf("command %q ran but should not have", tt.wantSkip)
			}
			if tt.wantCode == exitFailed && !strings.Contains(tg.stderr.String(), "sage") {
				t.Errorf("stderr = %q, want usage help", tg.stderr)
			}
		})
	}
}

func TestRunFailsWhenTheGateFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		failOn string
	}{
		{name: "verify", args: []string{"verify"}, failOn: "go vet"},
		{name: "doctor", args: []string{"doctor"}, failOn: "git config --get"},
		{name: "setup", args: []string{"setup"}, failOn: "git config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.healthyRepo(t, coveredProfile)
			tg.failOn(tt.failOn, "boom")

			if code := run(context.Background(), tt.args, tg.gate); code != exitFailed {
				t.Errorf("run(%v) = %d, want %d", tt.args, code, exitFailed)
			}
		})
	}
}

func TestRunFailsWhenOutputCannotBeWritten(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.healthyRepo(t, coveredProfile)
	tg.out = &printer{w: failingWriter{}}

	if code := run(context.Background(), []string{"verify", "--quick"}, tg.gate); code != exitFailed {
		t.Errorf("run() = %d, want %d when stdout is broken", code, exitFailed)
	}
}

func TestCommitMsgCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		message  string
		wantCode int
	}{
		{name: "conventional commit is accepted", message: "feat: add video feed\n", wantCode: exitOK},
		{name: "free-form message is rejected", message: "did some work\n", wantCode: exitFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			file := filepath.Join(tg.root, "COMMIT_EDITMSG")
			writeFile(t, file, tt.message)

			if code := run(context.Background(), []string{"commit-msg", file}, tg.gate); code != tt.wantCode {
				t.Errorf("run() = %d, want %d (stderr %q)", code, tt.wantCode, tg.stderr)
			}
		})
	}
}

func TestCommitMsgCommandFailsOnMissingFile(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	missing := filepath.Join(tg.root, "nope")

	if code := run(context.Background(), []string{"commit-msg", missing}, tg.gate); code != exitFailed {
		t.Errorf("run() = %d, want %d", code, exitFailed)
	}
}

func TestCommitsCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		log      string
		failGit  bool
		wantCode int
		wantArgs []string
	}{
		{name: "last commit by default", log: "feat: add feed\n", wantCode: exitOK, wantArgs: []string{"-n", "1"}},
		{name: "range is passed after end-of-options", args: []string{"origin/main..HEAD"}, log: "feat: a\nfix: b\n", wantCode: exitOK, wantArgs: []string{"--end-of-options", "origin/main..HEAD"}},
		{name: "one bad subject fails the range", args: []string{"origin/main..HEAD"}, log: "feat: a\nwip\n", wantCode: exitFailed},
		{name: "empty history passes", log: "\n", wantCode: exitOK},
		{name: "git failure fails", failGit: true, wantCode: exitFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.fake.reply = func(string, command) (string, error) { return tt.log, nil }
			if tt.failGit {
				tg.failOn("git log", "bad revision")
			}

			args := append([]string{"commits"}, tt.args...)
			if code := run(context.Background(), args, tg.gate); code != tt.wantCode {
				t.Errorf("run(%v) = %d, want %d (stderr %q)", args, code, tt.wantCode, tg.stderr)
			}
			for _, want := range tt.wantArgs {
				if !slices.Contains(tg.fake.cmds[0].args, want) {
					t.Errorf("git args = %v, want them to include %q", tg.fake.cmds[0].args, want)
				}
			}
		})
	}
}

func TestStartUsesTheEnclosingModule(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := start(context.Background(), []string{"frobnicate"}, strings.NewReader(""), &stdout, &stderr)

	if code != exitFailed {
		t.Errorf("start() = %d, want %d", code, exitFailed)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("stderr = %q, want the gate's own usage error", &stderr)
	}
}

func TestStartFailsOutsideAModule(t *testing.T) {
	t.Chdir(t.TempDir())

	var stdout, stderr bytes.Buffer
	code := start(context.Background(), []string{"doctor"}, strings.NewReader(""), &stdout, &stderr)

	if code != exitFailed {
		t.Errorf("start() = %d, want %d", code, exitFailed)
	}
	if !strings.Contains(stderr.String(), errNoModule.Error()) {
		t.Errorf("stderr = %q, want %q", &stderr, errNoModule)
	}
}

func TestFindRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example\n")
	nested := filepath.Join(root, "internal", "deep")
	writeFile(t, filepath.Join(nested, "x.go"), "package deep\n")

	for _, start := range []string{root, nested} {
		got, err := findRoot(start)
		if err != nil {
			t.Fatalf("findRoot(%q) error = %v", start, err)
		}
		if got != root {
			t.Errorf("findRoot(%q) = %q, want %q", start, got, root)
		}
	}

	if _, err := findRoot(t.TempDir()); !errors.Is(err, errNoModule) {
		t.Errorf("findRoot() outside a module error = %v, want %v", err, errNoModule)
	}
}

func TestExecRunner(t *testing.T) {
	t.Parallel()

	out, err := execRunner{}.Run(context.Background(), command{name: "go", args: []string{"env", "GOBIN"}, env: []string{"GOBIN=" + t.TempDir()}})
	if err != nil {
		t.Fatalf("Run() error = %v\n%s", err, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("Run() did not pass the extra environment to the command")
	}

	if _, err := (execRunner{}).Run(context.Background(), command{name: "go", args: []string{"no-such-subcommand"}}); err == nil {
		t.Error("Run() error = nil for a failing command")
	}
}

func TestToolPathAddsExeOnWindows(t *testing.T) {
	t.Parallel()

	g := &gate{root: "repo"}
	want := filepath.Join("repo", ".tools", "bin", "govulncheck")
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got := g.toolPath("govulncheck"); got != want {
		t.Errorf("toolPath() = %q, want %q", got, want)
	}
}

func TestTailKeepsTheEndOfLongOutput(t *testing.T) {
	t.Parallel()

	lines := make([]string, maxReportLines+5)
	for i := range lines {
		lines[i] = "line"
	}
	lines[len(lines)-1] = "last"

	got := strings.Split(tail(strings.Join(lines, "\r\n")), "\n")
	if len(got) != maxReportLines+1 || got[0] != "..." || got[len(got)-1] != "last" {
		t.Errorf("tail() kept %d lines, first %q, last %q", len(got), got[0], got[len(got)-1])
	}
	if short := tail("  one\ntwo  "); short != "one\ntwo" {
		t.Errorf("tail() of short output = %q", short)
	}
}

func TestPrinterStopsAfterTheFirstError(t *testing.T) {
	t.Parallel()

	p := &printer{w: failingWriter{}}
	p.printf("first")
	first := p.err
	p.printf("second")

	if first == nil || !errors.Is(p.err, first) {
		t.Errorf("printer error = %v, want the first write error %v", p.err, first)
	}
}
