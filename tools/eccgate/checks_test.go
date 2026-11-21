package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyPassesOnHealthyRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		profile    profile
		wantCalled []string
		wantSkip   []string
	}{
		{
			name:       "quick skips the network checks",
			profile:    quick,
			wantCalled: []string{"gofmt -l", "go build", "go vet", "golangci-lint run", "go list", "go test -race", "git ls-files"},
			wantSkip:   []string{"go mod tidy", "govulncheck"},
		},
		{
			name:       "full runs everything",
			profile:    full,
			wantCalled: []string{"gofmt -l", "go mod tidy -diff", "go build", "golangci-lint run", "go test -race", "govulncheck"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.healthyRepo(t, coveredProfile)

			if !tg.verify(context.Background(), tt.profile, tg.out) {
				t.Fatalf("verify() = false, want true\n%s", tg.stdout)
			}
			if !strings.Contains(tg.stdout.String(), "RESULT: READY") {
				t.Errorf("report = %q, want it to announce READY", tg.stdout)
			}
			for _, prefix := range tt.wantCalled {
				if !tg.fake.called(prefix) {
					t.Errorf("command %q was not run; ran %v", prefix, tg.fake.calls)
				}
			}
			for _, prefix := range tt.wantSkip {
				if tg.fake.called(prefix) {
					t.Errorf("command %q ran in the %s profile", prefix, tt.profile)
				}
			}
		})
	}
}

func TestVerifyReportsEachFailure(t *testing.T) {
	t.Parallel()

	secret := sampleSecrets()["AWS access key"]

	tests := []struct {
		name     string
		sabotage func(t *testing.T, tg *testGate)
		wantFail string
		wantText string
	}{
		{
			name:     "unformatted files",
			sabotage: func(_ *testing.T, tg *testGate) { tg.succeedWith("gofmt -l", "internal/app/app.go\n") },
			wantFail: "fmt", wantText: "internal/app/app.go",
		},
		{
			name:     "gofmt cannot parse a file",
			sabotage: func(_ *testing.T, tg *testGate) { tg.failOn("gofmt -l", "a.go:1:1: expected 'package'") },
			wantFail: "fmt", wantText: "expected 'package'",
		},
		{
			name:     "untidy module",
			sabotage: func(_ *testing.T, tg *testGate) { tg.failOn("go mod tidy", "diff go.mod") },
			wantFail: "mod", wantText: "diff go.mod",
		},
		{
			name:     "vet findings",
			sabotage: func(_ *testing.T, tg *testGate) { tg.failOn("go vet", "unreachable code") },
			wantFail: "vet", wantText: "unreachable code",
		},
		{
			name:     "lint findings",
			sabotage: func(_ *testing.T, tg *testGate) { tg.failOn("golangci-lint", "error returned is not wrapped") },
			wantFail: "lint", wantText: "not wrapped",
		},
		{
			name: "package without tests",
			sabotage: func(_ *testing.T, tg *testGate) {
				tg.succeedWith("go list", "example/app 2 1 0\nexample/store 3 0 0\n")
			},
			wantFail: "tests-exist", wantText: "example/store",
		},
		{
			name:     "go list fails",
			sabotage: func(_ *testing.T, tg *testGate) { tg.failOn("go list", "no Go files") },
			wantFail: "tests-exist", wantText: "no Go files",
		},
		{
			name:     "failing test",
			sabotage: func(_ *testing.T, tg *testGate) { tg.failOn("go test", "--- FAIL: TestFeed") },
			wantFail: "test", wantText: "--- FAIL: TestFeed",
		},
		{
			name: "coverage below the minimum",
			sabotage: func(t *testing.T, tg *testGate) {
				tg.healthyRepo(t, "mode: atomic\na.go:1.1,2.2 1 1\na.go:3.1,4.2 1 0\n")
			},
			wantFail: "test", wantText: "50.0% < 80.0%",
		},
		{
			name:     "corrupt coverage profile",
			sabotage: func(t *testing.T, tg *testGate) { tg.healthyRepo(t, "not a profile\n") },
			wantFail: "test", wantText: "no mode line",
		},
		{
			name: "hardcoded secret",
			sabotage: func(t *testing.T, tg *testGate) {
				writeFile(t, filepath.Join(tg.root, "config.go"), "package main\nconst k = \""+secret+"\"\n")
				tg.succeedWith("git ls-files", "config.go\x00go.mod\x00")
			},
			wantFail: "secrets", wantText: "config.go:2: AWS access key",
		},
		{
			name:     "git cannot list files",
			sabotage: func(_ *testing.T, tg *testGate) { tg.failOn("git ls-files", "not a git repository") },
			wantFail: "secrets", wantText: "not a git repository",
		},
		{
			name:     "known vulnerability",
			sabotage: func(_ *testing.T, tg *testGate) { tg.failOn("govulncheck", "GO-2026-0001") },
			wantFail: "vuln", wantText: "GO-2026-0001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tg := newTestGate(t, "")
			tg.healthyRepo(t, coveredProfile)
			tt.sabotage(t, tg)

			if tg.verify(context.Background(), full, tg.out) {
				t.Fatalf("verify() = true, want false\n%s", tg.stdout)
			}
			report := tg.stdout.String()
			if !strings.Contains(report, "FAIL  "+tt.wantFail) {
				t.Errorf("report does not mark %q as failed:\n%s", tt.wantFail, report)
			}
			if !strings.Contains(report, tt.wantText) {
				t.Errorf("report does not contain %q:\n%s", tt.wantText, report)
			}
			if !strings.Contains(report, "RESULT: NOT READY (1 failed)") {
				t.Errorf("report does not announce exactly one failure:\n%s", report)
			}
		})
	}
}

func TestVerifyStopsWhenTheBuildIsBroken(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.healthyRepo(t, coveredProfile)
	tg.failOn("go build", "undefined: Feed")

	if tg.verify(context.Background(), full, tg.out) {
		t.Fatal("verify() = true, want false")
	}
	if tg.fake.called("go vet") || tg.fake.called("go test") {
		t.Errorf("checks ran after a broken build: %v", tg.fake.calls)
	}
	if !strings.Contains(tg.stdout.String(), "remaining checks were skipped") {
		t.Errorf("report does not explain the skipped checks:\n%s", tg.stdout)
	}
}

func TestChecksNeedingToolsExplainHowToInstallThem(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "") // no tools installed

	err := checkLint(context.Background(), tg.gate)
	if err == nil || !strings.Contains(err.Error(), "go run ./tools/eccgate setup") {
		t.Errorf("checkLint() error = %v, want a hint to run setup", err)
	}
	if len(tg.fake.calls) != 0 {
		t.Errorf("a missing tool was still executed: %v", tg.fake.calls)
	}
}

func TestCheckTestFailsWithoutACoverageProfile(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "") // go test "passes" but writes no profile

	if err := checkTest(context.Background(), tg.gate); err == nil {
		t.Fatal("checkTest() error = nil, want an error for the missing profile")
	}
}

func TestCheckErrorsAreTyped(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	tg.healthyRepo(t, "mode: atomic\na.go:1.1,2.2 1 0\n")

	if err := checkTest(context.Background(), tg.gate); !errors.Is(err, errCoverageBelow) {
		t.Errorf("checkTest() error = %v, want errCoverageBelow", err)
	}

	tg.failOn("go test", "boom")
	if err := checkTest(context.Background(), tg.gate); !errors.Is(err, errTestsFailed) {
		t.Errorf("checkTest() error = %v, want errTestsFailed", err)
	}
}

func TestUntestedPackages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		listing string
		want    string
	}{
		{name: "package with internal tests", listing: "example/a 2 1 0\n", want: ""},
		{name: "package with external tests only", listing: "example/a 2 0 1\n", want: ""},
		{name: "package with no Go files", listing: "example/docs 0 0 0\n", want: ""},
		{name: "untested packages are listed", listing: "example/a 2 1 0\nexample/b 1 0 0\nexample/c 4 0 0\n", want: "example/b,example/c"},
		{name: "unexpected lines are ignored", listing: "go: downloading x\nexample/a x 0 0\n\n", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := strings.Join(untestedPackages(tt.listing), ","); got != tt.want {
				t.Errorf("untestedPackages() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProfileString(t *testing.T) {
	t.Parallel()

	if quick.String() != "quick" || full.String() != "full" {
		t.Errorf("profile names = %q, %q", quick, full)
	}
}

// succeedWith makes commands starting with prefix succeed with output, on
// top of the replies already configured.
func (tg *testGate) succeedWith(prefix, output string) {
	previous := tg.fake.reply
	tg.fake.reply = func(line string, c command) (string, error) {
		if strings.HasPrefix(line, prefix) {
			return output, nil
		}
		return previous(line, c)
	}
}
