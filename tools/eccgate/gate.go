package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Process exit codes. Agent hooks never signal a decision through the exit
// code; see hooks.go.
const (
	exitOK = iota
	exitFailed
)

// maxReportLines caps how much tool output a failed check prints.
const maxReportLines = 60

var errNoModule = errors.New("go.mod not found in this directory or any parent")

// gate holds everything a command needs: where the repository is, how to run
// external tools, and where to read and write.
type gate struct {
	root   string
	run    runner
	stdin  io.Reader
	out    *printer
	errOut *printer
}

// findRoot returns the nearest directory at or above start that has a go.mod.
func findRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", start, err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errNoModule
		}
		dir = parent
	}
}

func (g *gate) binDir() string {
	return filepath.Join(g.root, ".tools", "bin")
}

// toolPath is where setup installs the pinned build of a tool.
func (g *gate) toolPath(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(g.binDir(), name)
}

// must runs a command that passes only when it exits zero.
func (g *gate) must(ctx context.Context, name string, args ...string) error {
	out, err := g.run.Run(ctx, command{dir: g.root, name: name, args: args})
	if err != nil {
		return fmt.Errorf("%w\n%s", err, tail(out))
	}
	return nil
}

// runTool runs one of the pinned tools installed by setup.
func (g *gate) runTool(ctx context.Context, name string, args ...string) error {
	path := g.toolPath(name)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s is not installed (run: go run ./tools/eccgate setup): %w", name, err)
	}
	return g.must(ctx, path, args...)
}

// tail returns the last maxReportLines lines of s, trimmed.
func tail(s string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")), "\n")
	if len(lines) > maxReportLines {
		lines = append([]string{"..."}, lines[len(lines)-maxReportLines:]...)
	}
	return strings.Join(lines, "\n")
}

// indent prefixes every line of s so failure details nest under their check.
func indent(s string) string {
	return "        " + strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n        ")
}
