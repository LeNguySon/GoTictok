package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

var errFake = errors.New("exit status 1")

// fakeRunner records every command and answers from reply, keyed by the
// command line with the tool's directory and .exe suffix removed.
type fakeRunner struct {
	calls []string
	cmds  []command
	reply func(line string, c command) (string, error)
}

func (f *fakeRunner) Run(_ context.Context, c command) (string, error) {
	line := commandLine(c)
	f.calls = append(f.calls, line)
	f.cmds = append(f.cmds, c)
	if f.reply == nil {
		return "", nil
	}
	return f.reply(line, c)
}

func commandLine(c command) string {
	name := strings.TrimSuffix(path.Base(filepath.ToSlash(c.name)), ".exe")
	return strings.TrimSpace(name + " " + strings.Join(c.args, " "))
}

func (f *fakeRunner) called(prefix string) bool {
	for _, call := range f.calls {
		if strings.HasPrefix(call, prefix) {
			return true
		}
	}
	return false
}

// testGate is a gate wired to a fake runner and in-memory streams, rooted in
// a temporary module directory.
type testGate struct {
	*gate
	fake   *fakeRunner
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func newTestGate(t *testing.T, stdin string) *testGate {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example\n")

	fake := &fakeRunner{}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	return &testGate{
		gate: &gate{
			root:   root,
			run:    fake,
			stdin:  strings.NewReader(stdin),
			out:    &printer{w: stdout},
			errOut: &printer{w: stderr},
		},
		fake:   fake,
		stdout: stdout,
		stderr: stderr,
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		t.Fatalf("create directory for %s: %v", name, err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// installFakeTools makes every pinned tool look installed at its pinned version.
func (tg *testGate) installFakeTools(t *testing.T) {
	t.Helper()
	for _, tool := range pinnedTools() {
		writeFile(t, tg.toolPath(tool.name), "binary")
		writeFile(t, tg.stampPath(tool), tool.version+"\n")
	}
}

// coveredProfile is a cover profile with every statement covered.
const coveredProfile = "mode: atomic\nexample/a.go:1.1,2.2 4 1\n"

// healthyRepo answers every gate command the way a compliant repository
// would, writing coverage as the cover profile when go test runs.
func (tg *testGate) healthyRepo(t *testing.T, coverage string) {
	t.Helper()
	tg.installFakeTools(t)
	tg.fake.reply = func(line string, _ command) (string, error) {
		switch {
		case strings.HasPrefix(line, "go test"):
			writeFile(t, filepath.Join(tg.root, ".tools", "coverage.out"), coverage)
		case strings.HasPrefix(line, "go list"):
			return "example/app 2 1 0\n", nil
		case strings.HasPrefix(line, "go mod edit"):
			return `{"Module":{"Path":"example"}}`, nil
		case strings.HasPrefix(line, "git config --get"):
			return hooksDir + "\n", nil
		}
		return "", nil
	}
}

// failOn makes commands starting with prefix fail with output, on top of the
// replies already configured.
func (tg *testGate) failOn(prefix, output string) {
	previous := tg.fake.reply
	tg.fake.reply = func(line string, c command) (string, error) {
		if strings.HasPrefix(line, prefix) {
			return output, errFake
		}
		if previous == nil {
			return "", nil
		}
		return previous(line, c)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
