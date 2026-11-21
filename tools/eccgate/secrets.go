package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// maxScanBytes skips files too large to be hand-written source or config.
const maxScanBytes = 1 << 20

var errSecretsFound = errors.New("possible hardcoded secrets (move them to environment variables or a secret manager)")

// High-confidence credential shapes only: a false positive here blocks a
// commit, so vague "password = ..." heuristics are left to gosec (G101).
var (
	reAWSKey      = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	rePrivateKey  = regexp.MustCompile(`-----BEGIN (?:[A-Z]+ )?PRIVATE KEY-----`)
	reGitHubToken = regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`)
	reAPIKey      = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{24,}\b`)
	reSlackToken  = regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`)
	reGoogleKey   = regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)
)

type secretPattern struct {
	kind string
	re   *regexp.Regexp
}

func secretPatterns() []secretPattern {
	return []secretPattern{
		{"AWS access key", reAWSKey},
		{"private key", rePrivateKey},
		{"GitHub token", reGitHubToken},
		{"API secret key", reAPIKey},
		{"Slack token", reSlackToken},
		{"Google API key", reGoogleKey},
	}
}

type secretFinding struct {
	line int
	kind string
}

// findSecrets reports every line of data that looks like it holds a credential.
// Binary data is ignored.
func findSecrets(data []byte) []secretFinding {
	if bytes.IndexByte(data, 0) >= 0 {
		return nil
	}

	var findings []secretFinding
	for i, line := range bytes.Split(data, []byte("\n")) {
		for _, p := range secretPatterns() {
			if p.re.Match(line) {
				findings = append(findings, secretFinding{line: i + 1, kind: p.kind})
				break
			}
		}
	}
	return findings
}

// scanFile returns one report line per suspected secret in a repository file.
// Paths that no longer exist (deleted but still tracked) are skipped.
func (g *gate) scanFile(rel string) []string {
	path := filepath.Join(g.root, filepath.FromSlash(rel))
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxScanBytes {
		return nil
	}

	//nolint:gosec // G304: the path comes from `git ls-files` inside this repository.
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("%s: cannot be read: %v", rel, err)}
	}

	findings := findSecrets(data)
	reports := make([]string, 0, len(findings))
	for _, f := range findings {
		reports = append(reports, fmt.Sprintf("%s:%d: %s", rel, f.line, f.kind))
	}
	return reports
}
