package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// profile selects how much of the gate runs.
type profile int

const (
	// quick runs on every commit and when an agent tries to finish.
	quick profile = iota
	// full adds the network-dependent checks; it runs on push and in CI.
	full
)

func (p profile) String() string {
	if p == full {
		return "full"
	}
	return "quick"
}

var (
	errNotFormatted  = errors.New("files are not gofmt-formatted (run: gofmt -w .)")
	errUntested      = errors.New("packages have Go code but no tests (write the test first)")
	errTestsFailed   = errors.New("tests failed")
	errCoverageBelow = errors.New("coverage is below the required minimum")
)

// check is one rule of the ECC contract turned into a pass/fail command.
type check struct {
	name     string
	rule     string // the ECC rule the check enforces, shown in the report
	fullOnly bool
	fn       func(ctx context.Context, g *gate) error
}

// checks lists the gate in execution order. It mirrors the phases of ECC's
// verification-loop skill: build, static analysis, lint, tests, security.
func checks() []check {
	return []check{
		{"fmt", "gofmt is mandatory (golang/coding-style)", false, checkFmt},
		{"mod", "go.mod and go.sum are tidy", true, checkMod},
		{"deps", "every direct dependency has a recorded decision (project/research-first)", false, checkDeps},
		{"build", "the project builds (verification-loop)", false, checkBuild},
		{"vet", "go vet is clean (golang/hooks)", false, checkVet},
		{"lint", "golangci-lint: industrial standards (project/industrial-standards)", false, checkLint},
		{"tests-exist", "every package has tests (common/testing, TDD)", false, checkTestsExist},
		{"test", "go test -race passes with coverage >= 80% (golang/testing)", false, checkTest},
		{"secrets", "no hardcoded secrets (common/security)", false, checkSecrets},
		{"vuln", "no known vulnerabilities: govulncheck (go-reviewer)", true, checkVuln},
	}
}

// verify runs every check of the profile, writes a PASS/FAIL report to w and
// reports whether the tree is compliant.
func (g *gate) verify(ctx context.Context, p profile, w *printer) bool {
	w.printf("ECC gate (%s)\n", p)
	failed := 0
	for _, c := range checks() {
		if c.fullOnly && p != full {
			continue
		}
		err := c.fn(ctx, g)
		if err == nil {
			w.printf("  PASS  %-11s %s\n", c.name, c.rule)
			continue
		}
		failed++
		w.printf("  FAIL  %-11s %s\n%s\n", c.name, c.rule, indent(err.Error()))
		if c.name == "build" {
			w.printf("  the build is broken, so the remaining checks were skipped\n")
			break
		}
	}
	if failed > 0 {
		w.printf("RESULT: NOT READY (%d failed)\n", failed)
		return false
	}
	w.printf("RESULT: READY\n")
	return true
}

func checkFmt(ctx context.Context, g *gate) error {
	out, err := g.run.Run(ctx, command{dir: g.root, name: "gofmt", args: []string{"-l", "."}})
	if err != nil {
		return fmt.Errorf("%w\n%s", err, tail(out))
	}
	if files := strings.TrimSpace(out); files != "" {
		return fmt.Errorf("%w:\n%s", errNotFormatted, files)
	}
	return nil
}

func checkMod(ctx context.Context, g *gate) error {
	return g.must(ctx, "go", "mod", "tidy", "-diff")
}

func checkBuild(ctx context.Context, g *gate) error {
	return g.must(ctx, "go", "build", "./...")
}

func checkVet(ctx context.Context, g *gate) error {
	return g.must(ctx, "go", "vet", "./...")
}

func checkLint(ctx context.Context, g *gate) error {
	return g.runTool(ctx, "golangci-lint", "run", "./...")
}

func checkVuln(ctx context.Context, g *gate) error {
	return g.runTool(ctx, "govulncheck", "./...")
}

func checkTestsExist(ctx context.Context, g *gate) error {
	const format = "{{.ImportPath}} {{len .GoFiles}} {{len .TestGoFiles}} {{len .XTestGoFiles}}"
	out, err := g.run.Run(ctx, command{dir: g.root, name: "go", args: []string{"list", "-f", format, "./..."}})
	if err != nil {
		return fmt.Errorf("%w\n%s", err, tail(out))
	}
	if missing := untestedPackages(out); len(missing) > 0 {
		return fmt.Errorf("%w:\n%s", errUntested, strings.Join(missing, "\n"))
	}
	return nil
}

// listFields is the number of columns checkTestsExist asks `go list` for.
const listFields = 4

// untestedPackages reads "importpath goFiles testFiles xtestFiles" lines and
// returns the packages that have code but no test file at all.
func untestedPackages(listing string) []string {
	var missing []string
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) != listFields {
			continue
		}
		code, errCode := strconv.Atoi(fields[1])
		tests, errTests := strconv.Atoi(fields[2])
		xtests, errXTests := strconv.Atoi(fields[3])
		if errCode != nil || errTests != nil || errXTests != nil {
			continue
		}
		if code > 0 && tests+xtests == 0 {
			missing = append(missing, fields[0])
		}
	}
	return missing
}

func checkTest(ctx context.Context, g *gate) error {
	profilePath := filepath.Join(g.root, ".tools", "coverage.out")
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o750); err != nil {
		return fmt.Errorf("create coverage directory: %w", err)
	}

	args := []string{"test", "-race", "-covermode=atomic", "-coverprofile=" + profilePath, "./..."}
	if out, err := g.run.Run(ctx, command{dir: g.root, name: "go", args: args}); err != nil {
		return fmt.Errorf("%w: %w\n%s", errTestsFailed, err, tail(out))
	}

	//nolint:gosec // G304: the gate reads the coverage profile it has just asked go test to write.
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("read coverage profile: %w", err)
	}
	percent, err := coveragePercent(data)
	if err != nil {
		return fmt.Errorf("parse coverage profile: %w", err)
	}
	if percent < minCoverage {
		return fmt.Errorf("%w: %.1f%% < %.1f%%", errCoverageBelow, percent, minCoverage)
	}
	return nil
}

func checkSecrets(ctx context.Context, g *gate) error {
	args := []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard"}
	out, err := g.run.Run(ctx, command{dir: g.root, name: "git", args: args})
	if err != nil {
		return fmt.Errorf("list repository files: %w\n%s", err, tail(out))
	}

	var findings []string
	for _, rel := range strings.Split(out, "\x00") {
		if rel != "" {
			findings = append(findings, g.scanFile(rel)...)
		}
	}
	if len(findings) > 0 {
		return fmt.Errorf("%w:\n%s", errSecretsFound, strings.Join(findings, "\n"))
	}
	return nil
}
