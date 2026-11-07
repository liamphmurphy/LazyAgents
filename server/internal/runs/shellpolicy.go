package runs

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var readSedRange = regexp.MustCompile(`^[0-9]+(,[0-9]+)?p$`)

// Recognize a small shell language. Anything dynamic or unrecognized retains
// execution permission. In particular, never execute a shell to classify it.
func shellScopes(command string, depth int) []string {
	if depth > 4 {
		return executionScopes(command)
	}
	f, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil || len(f.Stmts) == 0 {
		return executionScopes(command)
	}
	var scopes []string
	add := func(scope string) {
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	var stmt func(*syntax.Stmt)
	stmt = func(s *syntax.Stmt) {
		if s.Background || s.Coprocess {
			add("tests:run")
			return
		}
		for _, r := range s.Redirs {
			// Discarding diagnostics is common in read-only searches. Other redirects
			// (including input/process substitutions) require execution permission.
			dest, ok := literalWord(r.Word)
			if !ok || dest != "/dev/null" || r.Op != syntax.RdrOut {
				add("tests:run")
			}
		}
		switch c := s.Cmd.(type) {
		case *syntax.BinaryCmd:
			if c.Op != syntax.AndStmt && c.Op != syntax.OrStmt && c.Op != syntax.Pipe {
				add("tests:run")
				return
			}
			stmt(c.X)
			stmt(c.Y)
		case *syntax.Subshell:
			for _, child := range c.Stmts {
				stmt(child)
			}
		case *syntax.CallExpr:
			if len(c.Assigns) != 0 {
				add("tests:run")
				return
			}
			args := make([]string, len(c.Args))
			for i, word := range c.Args {
				v, ok := literalWord(word)
				if !ok {
					add("tests:run")
					return
				}
				args[i] = v
			}
			for _, scope := range literalCommandScopes(args, depth) {
				add(scope)
			}
		default:
			add("tests:run")
		}
	}
	for _, s := range f.Stmts {
		stmt(s)
	}
	if len(scopes) == 0 {
		return []string{"tests:run"}
	}
	if slices.Contains(scopes, "tests:run") {
		for _, scope := range executionScopes(command) {
			add(scope)
		}
	}
	return scopes
}

func literalWord(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", false
	}
	var b strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar {
				return "", false
			}
			v, ok := literalWord(&syntax.Word{Parts: p.Parts})
			if !ok {
				return "", false
			}
			b.WriteString(v)
		default:
			return "", false
		}
	}
	return b.String(), true
}

func literalCommandScopes(args []string, depth int) []string {
	execution := []string{"tests:run"}
	if len(args) == 0 {
		return execution
	}
	// Only known system shell paths may wrap an otherwise recognized command.
	shell := args[0]
	if slices.Contains([]string{"sh", "bash", "zsh", "/bin/sh", "/bin/bash", "/bin/zsh", "/usr/bin/bash"}, shell) {
		if len(args) == 3 && (args[1] == "-c" || args[1] == "-lc") {
			return shellScopes(args[2], depth+1)
		}
		return execution
	}
	// Do not accept arbitrary executables merely because their basename matches.
	if filepath.Base(args[0]) != args[0] {
		return execution
	}
	read := []string{"repo:read"}
	switch args[0] {
	case "cat", "ls", "pwd", "grep", "head", "tail", "wc", "cd":
		return read
	case "sed":
		// Numeric print ranges only: sed's e/w/r commands and substitution
		// flags can execute programs or write files.
		if len(args) >= 3 && args[1] == "-n" && readSedRange.MatchString(args[2]) {
			for _, arg := range args[3:] {
				if strings.HasPrefix(arg, "-") {
					return execution
				}
			}
			return read
		}
		return execution
	case "rg":
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "--pre") || strings.HasPrefix(a, "--hostname-bin") {
				return execution
			}
		}
		return read
	case "git":
		i := 1
		if i < len(args) && args[i] == "--no-pager" {
			i++
		}
		// -C changes only the repository being inspected, not configuration/hooks.
		for i+1 < len(args) && args[i] == "-C" {
			i += 2
		}
		if i >= len(args) {
			return execution
		}
		switch args[i] {
		case "status", "diff", "log", "show", "rev-parse", "ls-files", "ls-tree", "grep":
			for _, a := range args[i+1:] {
				if strings.HasPrefix(a, "--output") || strings.HasPrefix(a, "--ext-diff") || strings.HasPrefix(a, "--textconv") || strings.HasPrefix(a, "-O") || strings.HasPrefix(a, "--open-files-in-pager") {
					return execution
				}
			}
			return read
		case "push":
			// Push hooks can execute arbitrary project code. Only the ordinary push
			// form is a shipping operation; custom options retain execution scope.
			for _, a := range args[i+1:] {
				if strings.HasPrefix(a, "-") && a != "--set-upstream" && a != "-u" && a != "--dry-run" {
					return []string{"tests:run", "repo:write"}
				}
			}
			return []string{"repo:write"}
		}
	}
	// Preserve repository-write enforcement for unrecognized execution commands.
	return executionScopes(strings.Join(args, " "))
}

func executionScopes(command string) []string {
	lower := strings.ToLower(command)
	if strings.Contains(lower, "git") && (strings.Contains(lower, "commit") || strings.Contains(lower, "push") || strings.Contains(lower, "checkout") || strings.Contains(lower, "reset")) || strings.Contains(lower, "gh") && (strings.Contains(lower, "create") || strings.Contains(lower, "merge") || strings.Contains(lower, "edit")) {
		return []string{"tests:run", "repo:write"}
	}
	return []string{"tests:run"}
}
