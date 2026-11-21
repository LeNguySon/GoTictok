package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hooksDir is the versioned directory git is pointed at for its hooks.
const hooksDir = ".githooks"

// tool is an external checker built from source at a pinned version, so every
// developer, agent and CI run uses exactly the same linter.
type tool struct {
	name    string
	pkg     string
	version string
}

func pinnedTools() []tool {
	return []tool{
		{"golangci-lint", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint", "v2.15.0"},
		{"govulncheck", "golang.org/x/vuln/cmd/govulncheck", "v1.10.0"},
	}
}

// setup prepares a clone: it activates the git hooks and installs the pinned
// tools into .tools/bin. It is safe to run repeatedly.
func (g *gate) setup(ctx context.Context, toolsOnly bool) error {
	if !toolsOnly {
		if err := g.ensureHooksPath(ctx); err != nil {
			return err
		}
		g.out.printf("git hooks  active (core.hooksPath = %s)\n", hooksDir)
		if err := g.registerGraphMerge(ctx); err != nil {
			return err
		}
		g.out.printf("graph      merge driver registered (merge.graphify)\n")
	}

	if err := os.MkdirAll(g.binDir(), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", g.binDir(), err)
	}
	for _, t := range pinnedTools() {
		state := "already installed"
		if !g.toolReady(t) {
			g.out.printf("%-10s building %s from source, this can take a few minutes...\n", t.name, t.version)
			if err := g.installTool(ctx, t); err != nil {
				return err
			}
			state = "installed"
		}
		g.out.printf("%-10s %s %s\n", t.name, t.version, state)
	}
	return nil
}

// registerGraphMerge teaches git to union-merge the committed knowledge graph
// (see .gitattributes), so branches that both rebuilt it do not conflict.
// Git settings are per clone, which is why this lives in setup.
func (g *gate) registerGraphMerge(ctx context.Context) error {
	settings := [][2]string{
		{"merge.graphify.name", "graphify graph.json union merge"},
		{"merge.graphify.driver", "graphify merge-driver %O %A %B"},
	}
	for _, s := range settings {
		out, err := g.run.Run(ctx, command{dir: g.root, name: "git", args: []string{"config", s[0], s[1]}})
		if err != nil {
			return fmt.Errorf("set %s: %w\n%s", s[0], err, tail(out))
		}
	}
	return nil
}

// ensureHooksPath points git at the versioned hooks directory.
func (g *gate) ensureHooksPath(ctx context.Context) error {
	out, err := g.run.Run(ctx, command{dir: g.root, name: "git", args: []string{"config", "core.hooksPath", hooksDir}})
	if err != nil {
		return fmt.Errorf("set core.hooksPath: %w\n%s", err, tail(out))
	}
	return nil
}

func (g *gate) stampPath(t tool) string {
	return filepath.Join(g.binDir(), t.name+".version")
}

// toolReady reports whether the pinned version of t is already installed.
func (g *gate) toolReady(t tool) bool {
	if _, err := os.Stat(g.toolPath(t.name)); err != nil {
		return false
	}
	stamp, err := os.ReadFile(g.stampPath(t))
	return err == nil && strings.TrimSpace(string(stamp)) == t.version
}

func (g *gate) installTool(ctx context.Context, t tool) error {
	out, err := g.run.Run(ctx, command{
		dir:  g.root,
		name: "go",
		args: []string{"install", t.pkg + "@" + t.version},
		env:  []string{"GOBIN=" + g.binDir()},
	})
	if err != nil {
		return fmt.Errorf("install %s %s: %w\n%s", t.name, t.version, err, tail(out))
	}
	if err := os.WriteFile(g.stampPath(t), []byte(t.version+"\n"), 0o600); err != nil {
		return fmt.Errorf("record %s version: %w", t.name, err)
	}
	return nil
}

// doctor reports whether this clone is ready to enforce the contract. Only
// the hooks and the pinned tools are required; Graphify is advisory.
func (g *gate) doctor(ctx context.Context, w *printer) bool {
	ready := true
	report := func(ok, required bool, label, fix string) {
		switch {
		case ok:
			w.printf("  ok       %s\n", label)
		case required:
			ready = false
			w.printf("  MISSING  %s (%s)\n", label, fix)
		default:
			w.printf("  warning  %s (%s)\n", label, fix)
		}
	}

	out, err := g.run.Run(ctx, command{dir: g.root, name: "git", args: []string{"config", "--get", "core.hooksPath"}})
	report(err == nil && strings.TrimSpace(out) == hooksDir, true, "git hooks active", "go run ./tools/eccgate setup")
	for _, t := range pinnedTools() {
		report(g.toolReady(t), true, t.name+" "+t.version, "go run ./tools/eccgate setup")
	}

	_, err = g.run.Run(ctx, command{dir: g.root, name: "graphify", args: []string{"--version"}})
	report(err == nil, false, "graphify CLI", "uv tool install graphifyy")
	_, err = os.Stat(filepath.Join(g.root, "graphify-out", "graph.json"))
	report(err == nil, false, "knowledge graph built", "graphify update .")
	return ready
}
