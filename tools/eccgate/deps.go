package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// dependencyLedger records, for every direct dependency, why it was chosen
// over the alternatives. It is the checkable outcome of the research rule in
// .claude/rules/project/research-first.md.
const dependencyLedger = "docs/research/dependencies.md"

var (
	errUnrecordedDeps = errors.New("direct dependencies have no recorded research decision")
	errNoModuleData   = errors.New("go mod edit printed no module description")
)

// moduleFile is the part of `go mod edit -json` the gate reads.
type moduleFile struct {
	Require []struct {
		Path     string `json:"Path"`
		Indirect bool   `json:"Indirect"`
	} `json:"Require"`
}

// checkDeps fails when go.mod requires a module directly that the ledger does
// not name. An agent cannot add a dependency without writing down what it
// compared it against.
func checkDeps(ctx context.Context, g *gate) error {
	out, err := g.run.Run(ctx, command{dir: g.root, name: "go", args: []string{"mod", "edit", "-json"}})
	if err != nil {
		return fmt.Errorf("%w\n%s", err, tail(out))
	}
	direct, err := directDependencies(out)
	if err != nil {
		return err
	}
	if len(direct) == 0 {
		return nil
	}

	ledger, err := os.ReadFile(g.ledgerPath())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", dependencyLedger, err)
	}
	if missing := unrecorded(direct, string(ledger)); len(missing) > 0 {
		return fmt.Errorf("%w: research the alternatives, then add each one to %s:\n%s",
			errUnrecordedDeps, dependencyLedger, strings.Join(missing, "\n"))
	}
	return nil
}

func (g *gate) ledgerPath() string {
	return filepath.Join(g.root, filepath.FromSlash(dependencyLedger))
}

// directDependencies extracts the direct requirements from the output of
// `go mod edit -json`. Empty output is an error: the check fails closed.
func directDependencies(modJSON string) ([]string, error) {
	if strings.TrimSpace(modJSON) == "" {
		return nil, errNoModuleData
	}
	var mod moduleFile
	if err := json.Unmarshal([]byte(modJSON), &mod); err != nil {
		return nil, fmt.Errorf("parse go mod edit output: %w", err)
	}

	direct := make([]string, 0, len(mod.Require))
	for _, requirement := range mod.Require {
		if !requirement.Indirect {
			direct = append(direct, requirement.Path)
		}
	}
	return direct, nil
}

// unrecorded returns the modules the ledger does not name. A module counts as
// recorded when its exact path appears as a code span, which a longer or a
// shorter path cannot satisfy by accident.
func unrecorded(modules []string, ledger string) []string {
	var missing []string
	for _, module := range modules {
		if !strings.Contains(ledger, "`"+module+"`") {
			missing = append(missing, module)
		}
	}
	return missing
}
