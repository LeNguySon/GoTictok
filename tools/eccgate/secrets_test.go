package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The samples are assembled at run time so this file never contains a string
// that the scanner itself would report.
func sampleSecrets() map[string]string {
	return map[string]string{
		"AWS access key": "AKIA" + strings.Repeat("A", 16),
		"private key":    "-----BEGIN RSA " + "PRIVATE KEY-----",
		"GitHub token":   "ghp_" + strings.Repeat("a", 36),
		"API secret key": "sk-" + strings.Repeat("a", 30),
		"Slack token":    "xoxb-" + strings.Repeat("1", 12),
		"Google API key": "AIza" + strings.Repeat("a", 35),
	}
}

func TestFindSecretsDetectsEachKind(t *testing.T) {
	t.Parallel()

	for kind, secret := range sampleSecrets() {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			data := []byte("package config\n\nconst key = \"" + secret + "\"\n")
			findings := findSecrets(data)

			if len(findings) != 1 {
				t.Fatalf("findSecrets() = %v, want exactly one finding", findings)
			}
			if findings[0].kind != kind || findings[0].line != 3 {
				t.Errorf("findSecrets() = %+v, want kind %q on line 3", findings[0], kind)
			}
		})
	}
}

func TestFindSecretsIgnoresCleanAndBinaryData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
	}{
		{name: "ordinary code", data: []byte("apiKey := os.Getenv(\"API_KEY\")\n")},
		{name: "word ending in sk", data: []byte("task-management-and-scheduling-service-name\n")},
		{name: "short key-like text", data: []byte("sk-short\n")},
		{name: "binary file", data: append([]byte{0}, []byte(sampleSecrets()["AWS access key"])...)},
		{name: "empty", data: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if findings := findSecrets(tt.data); len(findings) != 0 {
				t.Errorf("findSecrets() = %v, want none", findings)
			}
		})
	}
}

func TestScanFile(t *testing.T) {
	t.Parallel()

	tg := newTestGate(t, "")
	secret := sampleSecrets()["GitHub token"]
	writeFile(t, filepath.Join(tg.root, "config", "prod.env"), "A=1\nTOKEN="+secret+"\n")
	writeFile(t, filepath.Join(tg.root, "clean.txt"), "nothing here\n")
	writeFile(t, filepath.Join(tg.root, "huge.bin"), strings.Repeat("x", maxScanBytes+1)+secret)

	tests := []struct {
		name string
		rel  string
		want []string
	}{
		{name: "reports file, line and kind", rel: "config/prod.env", want: []string{"config/prod.env:2: GitHub token"}},
		{name: "clean file", rel: "clean.txt"},
		{name: "deleted file is skipped", rel: "gone.txt"},
		{name: "directory is skipped", rel: "config"},
		{name: "oversized file is skipped", rel: "huge.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tg.scanFile(tt.rel)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("scanFile(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}
