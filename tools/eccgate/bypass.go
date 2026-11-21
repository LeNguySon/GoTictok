package main

import (
	"path"
	"slices"
	"strings"
)

// protectedPaths are the files that make up the gate itself. Entries ending
// in "/" are directories. Agents may not change them without a human saying
// yes, otherwise "passing the gate" could be achieved by weakening it.
func protectedPaths() []string {
	return []string{
		"agents.md",
		".golangci.yml",
		".githooks/",
		".github/workflows/",
		"tools/eccgate/",
		".claude/settings.json",
		".claude/rules/",
	}
}

// isProtected reports whether a file path, relative or absolute, points into
// the gate. Matching is by path suffix and ignores case, so it works for
// Windows, POSIX and MSYS spellings of the same file.
func isProtected(file string) bool {
	p := strings.ToLower(strings.ReplaceAll(file, `\`, "/"))
	p = strings.TrimPrefix(p, "./")
	for _, protected := range protectedPaths() {
		if dir, ok := strings.CutSuffix(protected, "/"); ok {
			if p == dir || strings.HasPrefix(p, protected) || strings.Contains(p, "/"+protected) || strings.HasSuffix(p, "/"+dir) {
				return true
			}
			continue
		}
		if p == protected || strings.HasSuffix(p, "/"+protected) {
			return true
		}
	}
	return false
}

// splitShell splits a command line into simple commands, each a list of
// words. It understands quotes and the separators ; & | ( ) and newline:
// enough to recognise git invocations, not a full shell parser.
func splitShell(line string) [][]string {
	var (
		commands [][]string
		words    []string
		word     strings.Builder
		quote    rune
		inWord   bool
	)
	endWord := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}
	endCommand := func() {
		endWord()
		if len(words) > 0 {
			commands = append(commands, words)
			words = nil
		}
	}

	for _, r := range line {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			endWord()
		case strings.ContainsRune(";&|()\n", r):
			endCommand()
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	endCommand()
	return commands
}

// bypassReason explains why a shell command would skip the git hooks, or
// returns "" when it would not.
func bypassReason(line string) string {
	for _, words := range splitShell(line) {
		if reason := gitBypass(words); reason != "" {
			return reason
		}
	}
	return ""
}

func isGit(word string) bool {
	base := strings.ToLower(path.Base(strings.ReplaceAll(word, `\`, "/")))
	return base == "git" || base == "git.exe"
}

func gitBypass(words []string) string {
	start := slices.IndexFunc(words, isGit)
	if start < 0 {
		return ""
	}
	args := words[start+1:]

	for _, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "--no-verify" {
			return "--no-verify skips the git hooks"
		}
		if strings.HasPrefix(lower, "core.hookspath=") {
			return "overriding core.hooksPath disables the git hooks"
		}
	}

	positional := positionalArgs(args)
	switch {
	case len(positional) == 0:
		return ""
	case positional[0] == "commit" && hasShortNoVerify(args):
		return "git commit -n skips the git hooks"
	case positional[0] == "config" && changesHooksPath(args, positional[1:]):
		return "changing core.hooksPath disables the git hooks"
	}
	return ""
}

// positionalArgs drops flags, keeping the subcommand and its operands. The
// value that follows -C or -c (a directory, a config entry or a commit) is
// dropped with its flag, so `git -C dir config ...` still starts with "config".
func positionalArgs(args []string) []string {
	var positional []string
	skipNext := false
	for _, arg := range args {
		switch {
		case skipNext:
			skipNext = false
		case arg == "-C" || arg == "-c":
			skipNext = true
		case !strings.HasPrefix(arg, "-"):
			positional = append(positional, arg)
		}
	}
	return positional
}

// hasShortNoVerify finds -n inside a short-flag cluster such as -nm or -an.
// Scanning stops at a flag that takes a value, because the rest of the
// cluster is then that value (as in -mnotes).
func hasShortNoVerify(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
			continue
		}
		for _, flag := range arg[1:] {
			if flag == 'n' {
				return true
			}
			if strings.ContainsRune("mFCctSu", flag) {
				break
			}
		}
	}
	return false
}

// changesHooksPath reports whether a `git config` call writes or removes
// core.hooksPath. Merely reading the value is allowed.
func changesHooksPath(args, operands []string) bool {
	key := slices.IndexFunc(operands, func(s string) bool { return strings.EqualFold(s, "core.hooksPath") })
	if key < 0 {
		return false
	}
	writes := func(s string) bool {
		return slices.Contains([]string{"--unset", "--unset-all", "--add", "--replace-all"}, s)
	}
	if slices.ContainsFunc(args, writes) {
		return true
	}
	// `git config set|unset core.hooksPath` or `git config core.hooksPath <value>`.
	usesWriteVerb := key > 0 && operands[0] != "get"
	hasValue := key < len(operands)-1
	return usesWriteVerb || hasValue
}

// mutatedProtectedPath returns the gate file a shell command would write to,
// move or delete, or "" when it touches none.
func mutatedProtectedPath(line string) string {
	for _, words := range splitShell(line) {
		if target := protectedTarget(words); target != "" {
			return target
		}
	}
	return ""
}

func protectedTarget(words []string) string {
	mutating := isMutatingCommand(words)
	redirected := false
	for _, word := range words {
		target := word
		if i := strings.LastIndex(word, ">"); i >= 0 {
			redirected, target = true, word[i+1:]
		}
		if target == "" {
			continue
		}
		if (mutating || redirected) && isProtected(target) {
			return target
		}
		redirected = false
	}
	return ""
}

// isMutatingCommand recognises the common ways a shell rewrites, moves or
// removes a file. It is a tripwire, not a sandbox: CI remains the authority.
func isMutatingCommand(words []string) bool {
	name := strings.ToLower(path.Base(strings.ReplaceAll(words[0], `\`, "/")))
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "rm", "mv", "cp", "tee", "chmod", "truncate", "del", "rd", "rmdir", "ln", "dd", "patch",
		"remove-item", "move-item", "copy-item", "set-content", "add-content", "out-file", "clear-content":
		return true
	case "sed", "perl":
		return slices.ContainsFunc(words, func(w string) bool { return strings.HasPrefix(w, "-i") })
	case "git":
		positional := positionalArgs(words[1:])
		return len(positional) > 0 && slices.Contains([]string{"rm", "mv", "checkout", "restore"}, positional[0])
	}
	return false
}
