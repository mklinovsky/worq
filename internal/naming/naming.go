// Package naming turns what you typed into a branch and a directory name.
package naming

import (
	"regexp"
	"strings"
)

var (
	jiraKey  = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]+)-(\d+)$`)
	bareNum  = regexp.MustCompile(`^#?(\d+)$`)
	nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
)

// Result is a branch name plus the directory name it maps to.
type Result struct {
	Branch string
	Dir    string
}

// Slug lowercases and hyphenates, keeping it readable.
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlnum.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// DirName maps a branch name to a directory name: feat/login -> feat-login.
func DirName(branch string) string {
	return strings.Trim(strings.ReplaceAll(branch, "/", "-"), "-")
}

// Build derives branch and directory names from the words given on the command
// line.
//
//	Build([]string{"EFRON-123", "fix", "login"}, "", "")   -> efron-123-fix-login
//	Build([]string{"123", "fix login"}, "EFRON", "")       -> efron-123-fix-login
//	Build([]string{"fix login"}, "", "feat/")              -> feat/fix-login
//	Build([]string{"feat/mobile/login"}, "", "feat/")      -> feat/mobile/login (kept as typed)
func Build(args []string, jiraProject, prefix string) Result {
	if len(args) == 0 {
		return Result{}
	}
	head := strings.TrimSpace(args[0])
	rest := strings.Join(args[1:], " ")

	// An explicit branch path is taken literally; you meant it.
	if strings.Contains(head, "/") && len(args) == 1 {
		return Result{Branch: head, Dir: DirName(head)}
	}

	if m := jiraKey.FindStringSubmatch(head); m != nil {
		key := strings.ToUpper(m[1]) + "-" + m[2]
		return withIssue(key, rest)
	}
	if m := bareNum.FindStringSubmatch(head); m != nil && jiraProject != "" {
		key := strings.ToUpper(jiraProject) + "-" + m[1]
		return withIssue(key, rest)
	}

	slug := Slug(strings.Join(args, " "))
	branch := prefix + slug
	return Result{Branch: branch, Dir: DirName(branch)}
}

func withIssue(key, summary string) Result {
	branch := strings.ToLower(key)
	if s := Slug(summary); s != "" {
		branch += "-" + s
	}
	return Result{Branch: branch, Dir: DirName(branch)}
}
