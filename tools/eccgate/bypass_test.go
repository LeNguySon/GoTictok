package main

import "testing"

func TestBypassReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		blocked bool
	}{
		{name: "plain commit", command: `git commit -m "feat: add feed"`},
		{name: "commit all with message", command: `git commit -am "feat: add feed"`},
		{name: "message that mentions -n", command: `git commit -m "docs: explain -n and --no-verify"`},
		{name: "message attached to -m", command: `git commit -mnotes`},
		{name: "push dry run", command: `git push -n origin main`},
		{name: "reading hooks path", command: `git config --get core.hooksPath`},
		{name: "reading hooks path without flag", command: `git config core.hooksPath`},
		{name: "reading hooks path with get verb", command: `git config get core.hooksPath`},
		{name: "unrelated config", command: `git config user.name "Dev"`},
		{name: "quoted git command is only text", command: `echo "git commit --no-verify"`},
		{name: "not git at all", command: `go test -race ./...`},
		{name: "bare git", command: `git`},
		{name: "empty", command: ``},

		{name: "commit --no-verify", command: `git commit --no-verify -m "x"`, blocked: true},
		{name: "commit -n", command: `git commit -n -m "x"`, blocked: true},
		{name: "commit -n inside a flag cluster", command: `git commit -anm "x"`, blocked: true},
		{name: "push --no-verify", command: `git push --no-verify`, blocked: true},
		{name: "chained after another command", command: `cd app && git commit --no-verify -m "x"`, blocked: true},
		{name: "second line of a script", command: "go build ./...\ngit commit -n -m x", blocked: true},
		{name: "hooks path override on the command line", command: `git -c core.hooksPath=/dev/null commit -m "x"`, blocked: true},
		{name: "setting hooks path", command: `git config core.hooksPath /tmp/none`, blocked: true},
		{name: "setting hooks path with set verb", command: `git config set core.hooksPath /tmp/none`, blocked: true},
		{name: "unsetting hooks path", command: `git config --unset core.hooksPath`, blocked: true},
		{name: "unsetting hooks path with unset verb", command: `git config unset core.hooksPath`, blocked: true},
		{name: "hooks path in another directory", command: `git -C sub config core.hookspath none`, blocked: true},
		{name: "full path to git on windows", command: `"C:\Program Files\Git\bin\git.exe" commit --no-verify`, blocked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reason := bypassReason(tt.command)
			if got := reason != ""; got != tt.blocked {
				t.Errorf("bypassReason(%q) = %q, blocked = %v, want %v", tt.command, reason, got, tt.blocked)
			}
		})
	}
}

func TestIsProtected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{path: "AGENTS.md", want: true},
		{path: ".golangci.yml", want: true},
		{path: "./.golangci.yml", want: true},
		{path: ".githooks", want: true},
		{path: ".githooks/pre-commit", want: true},
		{path: `C:\repo\.githooks\pre-commit`, want: true},
		{path: "/c/repo/tools/eccgate/main.go", want: true},
		{path: "/c/repo/tools/eccgate", want: true},
		{path: ".github/workflows/ci.yml", want: true},
		{path: ".claude/settings.json", want: true},
		{path: ".claude/rules/ecc/common/testing.md", want: true},
		{path: `C:\Repo\.GOLANGCI.YML`, want: true},
		{path: "internal/app/app.go", want: false},
		{path: "README.md", want: false},
		{path: ".claude/settings.local.json", want: false},
		{path: "docs/golangci.yml", want: false},
		{path: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			if got := isProtected(tt.path); got != tt.want {
				t.Errorf("isProtected(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMutatedProtectedPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    string
	}{
		{name: "delete a hook", command: `rm .githooks/pre-commit`, want: ".githooks/pre-commit"},
		{name: "redirect into the lint config", command: `echo "version: 2" > .golangci.yml`, want: ".golangci.yml"},
		{name: "redirect without a space", command: `echo x >.golangci.yml`, want: ".golangci.yml"},
		{name: "append redirect", command: `echo x >> AGENTS.md`, want: "AGENTS.md"},
		{name: "in-place sed on the gate", command: `sed -i s/80/10/ tools/eccgate/coverage.go`, want: "tools/eccgate/coverage.go"},
		{name: "git checkout of a gate file", command: `git checkout -- .golangci.yml`, want: ".golangci.yml"},
		{name: "powershell removal", command: `Remove-Item .githooks/pre-commit`, want: ".githooks/pre-commit"},
		{name: "second command in a chain", command: `go build ./... && rm -rf .githooks`, want: ".githooks"},
		{name: "reading a gate file", command: `cat AGENTS.md`},
		{name: "printing with sed", command: `sed -n 1,5p .golangci.yml`},
		{name: "redirect elsewhere", command: `go test ./... > out.txt`},
		{name: "deleting an ordinary file", command: `rm internal/app/old.go`},
		{name: "git status", command: `git status`},
		{name: "bare git", command: `git`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := mutatedProtectedPath(tt.command); got != tt.want {
				t.Errorf("mutatedProtectedPath(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}
