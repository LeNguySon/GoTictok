package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// buildTools makes the fake `go install` drop a binary into GOBIN, as the
// real command would.
func (tg *testGate) buildTools(t *testing.T) {
	t.Helper()
	tg.fake.reply = func(line string, _ command) (string, error) {
		for _, tool := range pinnedTools() {
			if strings.HasPrefix(line, "go install "+tool.pkg) {
				writeFile(t, tg.toolPath(tool.name), "binary")
			}
		}
		return "", nil
	}
}

func TestSetupInstallsOnceAndIsRepeatable(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.buildTools(t)

	if err := tg.setup(context.Background(), false); err != nil {
		t.Fatalf("first setup() error = %v", err)
	}
	if !tg.fake.called("git config core.hooksPath " + hooksDir) {
		t.Errorf("git hooks were not activated: %v", tg.fake.calls)
	}
	for _, tool := range pinnedTools() {
		if !tg.fake.called("go install " + tool.pkg + "@" + tool.version) {
			t.Errorf("%s was not installed at its pinned version: %v", tool.name, tg.fake.calls)
		}
		if !tg.toolReady(tool) {
			t.Errorf("%s is not ready after setup", tool.name)
		}
	}
	for _, c := range tg.fake.cmds {
		if c.name == "go" && (len(c.env) != 1 || c.env[0] != "GOBIN="+tg.binDir()) {
			t.Errorf("go install env = %v, want GOBIN pointing at %s", c.env, tg.binDir())
		}
	}

	installs := len(tg.fake.calls)
	if err := tg.setup(context.Background(), false); err != nil {
		t.Fatalf("second setup() error = %v", err)
	}
	if got := len(tg.fake.calls) - installs; got != gitConfigCalls {
		t.Errorf("second setup ran %d command(s), want only the %d git config calls: %v",
			got, gitConfigCalls, tg.fake.calls[installs:])
	}
	if !strings.Contains(tg.stdout.String(), "already installed") {
		t.Errorf("stdout = %q, want it to report tools as already installed", tg.stdout)
	}
}

// gitConfigCalls is what setup always runs: the hooks path plus the two
// settings of the knowledge-graph merge driver.
const gitConfigCalls = 3

func TestSetupRegistersTheGraphMergeDriver(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.buildTools(t)

	if err := tg.setup(context.Background(), false); err != nil {
		t.Fatalf("setup() error = %v", err)
	}
	if !tg.fake.called("git config merge.graphify.driver graphify merge-driver %O %A %B") {
		t.Errorf("merge driver was not registered: %v", tg.fake.calls)
	}
	if !tg.fake.called("git config merge.graphify.name ") {
		t.Errorf("merge driver was not named: %v", tg.fake.calls)
	}
}

func TestSetupFailsWhenTheMergeDriverCannotBeRegistered(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.failOn("git config merge.graphify", "locked")

	err := tg.setup(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "set merge.graphify.name") {
		t.Errorf("setup() error = %v, want it to mention the merge driver", err)
	}
}

func TestSetupToolsOnlyLeavesGitAlone(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.buildTools(t)

	if err := tg.setup(context.Background(), true); err != nil {
		t.Fatalf("setup() error = %v", err)
	}
	if tg.fake.called("git") {
		t.Errorf("tools-only setup touched git: %v", tg.fake.calls)
	}
}

func TestSetupReinstallsWhenThePinnedVersionChanges(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.installFakeTools(t)
	stale := pinnedTools()[0]
	writeFile(t, tg.stampPath(stale), "v0.0.1\n")
	tg.buildTools(t)

	if err := tg.setup(context.Background(), true); err != nil {
		t.Fatalf("setup() error = %v", err)
	}
	if !tg.fake.called("go install " + stale.pkg) {
		t.Errorf("stale %s was not reinstalled: %v", stale.name, tg.fake.calls)
	}
	if len(tg.fake.calls) != 1 {
		t.Errorf("ran %v, want only the stale tool reinstalled", tg.fake.calls)
	}
}

func TestSetupReportsFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		failOn   string
		wantText string
	}{
		{name: "git refuses the hooks path", failOn: "git config", wantText: "set core.hooksPath"},
		{name: "a tool does not build", failOn: "go install", wantText: "install golangci-lint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.failOn(tt.failOn, "boom")

			err := tg.setup(context.Background(), false)
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("setup() error = %v, want it to mention %q", err, tt.wantText)
			}
		})
	}
}

func TestDoctor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		prepare   func(t *testing.T, tg *testGate)
		wantReady bool
		wantText  string
	}{
		{
			name:      "fully set up",
			prepare:   func(t *testing.T, tg *testGate) { tg.healthyRepo(t, coveredProfile) },
			wantReady: true, wantText: "ok       git hooks active",
		},
		{
			name: "graph present is reported",
			prepare: func(t *testing.T, tg *testGate) {
				tg.healthyRepo(t, coveredProfile)
				writeFile(t, filepath.Join(tg.root, "graphify-out", "graph.json"), "{}")
			},
			wantReady: true, wantText: "ok       knowledge graph built",
		},
		{
			name: "missing graphify is only a warning",
			prepare: func(t *testing.T, tg *testGate) {
				tg.healthyRepo(t, coveredProfile)
				tg.failOn("graphify", "command not found")
			},
			wantReady: true, wantText: "warning  graphify CLI",
		},
		{
			name: "hooks not active",
			prepare: func(t *testing.T, tg *testGate) {
				tg.healthyRepo(t, coveredProfile)
				tg.succeedWith("git config --get", ".git/hooks\n")
			},
			wantReady: false, wantText: "MISSING  git hooks active",
		},
		{
			name:      "tools not installed",
			prepare:   func(_ *testing.T, tg *testGate) { tg.succeedWith("git config --get", hooksDir+"\n") },
			wantReady: false, wantText: "MISSING  golangci-lint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.fake.reply = func(string, command) (string, error) { return "", nil }
			tt.prepare(t, tg)

			if got := tg.doctor(context.Background(), tg.out); got != tt.wantReady {
				t.Errorf("doctor() = %v, want %v\n%s", got, tt.wantReady, tg.stdout)
			}
			if !strings.Contains(tg.stdout.String(), tt.wantText) {
				t.Errorf("report = %q, want it to contain %q", tg.stdout, tt.wantText)
			}
		})
	}
}
