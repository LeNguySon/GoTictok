package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

const (
	routerModule = "github.com/go-chi/chi/v5"
	driverModule = "github.com/jackc/pgx/v5"
)

// modJSON builds the output of `go mod edit -json` for the given requirements.
func modJSON(direct, indirect []string) string {
	entries := make([]string, 0, len(direct)+len(indirect))
	for _, m := range direct {
		entries = append(entries, `{"Path":"`+m+`","Version":"v1.0.0"}`)
	}
	for _, m := range indirect {
		entries = append(entries, `{"Path":"`+m+`","Version":"v1.0.0","Indirect":true}`)
	}
	return `{"Module":{"Path":"example"},"Require":[` + strings.Join(entries, ",") + `]}`
}

func TestCheckDeps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		module      string
		ledger      string // empty means the ledger file does not exist
		wantMissing []string
	}{
		{name: "no dependencies needs no ledger", module: modJSON(nil, nil)},
		{name: "module without a require block", module: `{"Module":{"Path":"example"}}`},
		{name: "indirect dependencies are exempt", module: modJSON(nil, []string{"golang.org/x/sys"})},
		{
			name:   "recorded dependency passes",
			module: modJSON([]string{routerModule}, nil),
			ledger: "| `" + routerModule + "` | HTTP router | MIT |\n",
		},
		{
			name:        "dependency without a ledger fails",
			module:      modJSON([]string{routerModule}, nil),
			wantMissing: []string{routerModule},
		},
		{
			name:        "only the unrecorded dependency is reported",
			module:      modJSON([]string{routerModule, driverModule}, []string{"golang.org/x/sys"}),
			ledger:      "| `" + routerModule + "` | HTTP router | MIT |\n",
			wantMissing: []string{driverModule},
		},
		{
			name:        "a longer recorded path does not cover a shorter module",
			module:      modJSON([]string{"github.com/go-chi/chi"}, nil),
			ledger:      "| `" + routerModule + "` | HTTP router | MIT |\n",
			wantMissing: []string{"github.com/go-chi/chi"},
		},
		{
			name:        "a mention outside a code span does not count",
			module:      modJSON([]string{routerModule}, nil),
			ledger:      "We might use " + routerModule + " one day.\n",
			wantMissing: []string{routerModule},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.fake.reply = func(string, command) (string, error) { return tt.module, nil }
			if tt.ledger != "" {
				writeFile(t, filepath.Join(tg.root, filepath.FromSlash(dependencyLedger)), tt.ledger)
			}

			err := checkDeps(context.Background(), tg.gate)

			if len(tt.wantMissing) == 0 {
				if err != nil {
					t.Fatalf("checkDeps() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, errUnrecordedDeps) {
				t.Fatalf("checkDeps() error = %v, want %v", err, errUnrecordedDeps)
			}
			for _, m := range tt.wantMissing {
				if !strings.Contains(err.Error(), "\n"+m) {
					t.Errorf("error does not list %q:\n%v", m, err)
				}
			}
			if strings.Count(err.Error(), "\n") != len(tt.wantMissing) {
				t.Errorf("error lists more than the %d missing module(s):\n%v", len(tt.wantMissing), err)
			}
		})
	}
}

func TestCheckDepsFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		failGo  bool
		wantErr error
	}{
		{name: "go mod edit fails", output: "go.mod: malformed", failGo: true, wantErr: errFake},
		{name: "empty output", output: "  \n", wantErr: errNoModuleData},
		{name: "output is not JSON", output: "go: warning"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.fake.reply = func(string, command) (string, error) {
				if tt.failGo {
					return tt.output, errFake
				}
				return tt.output, nil
			}

			err := checkDeps(context.Background(), tg.gate)
			if err == nil {
				t.Fatal("checkDeps() error = nil, want the check to fail closed")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("checkDeps() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckDepsIsPartOfTheQuickGate(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.healthyRepo(t, coveredProfile)
	tg.succeedWith("go mod edit", modJSON([]string{driverModule}, nil))

	if tg.verify(context.Background(), quick, tg.out) {
		t.Fatalf("verify() = true with an unrecorded dependency\n%s", tg.stdout)
	}
	if !strings.Contains(tg.stdout.String(), "FAIL  deps") {
		t.Errorf("report does not mark deps as failed:\n%s", tg.stdout)
	}
}
