package cli

import (
	"regexp"
	"strings"
	"testing"
)

func TestTableAlignsStyledCells(t *testing.T) {
	var b strings.Builder
	table(&b, [][]string{
		{"", "\x1b[2mNAME\x1b[0m", "\x1b[2mBRANCH\x1b[0m"},
		{"\x1b[32m*\x1b[0m", "current", "main"},
		{" ", "other", "\x1b[38;2;90;209;155mfeature\x1b[0m"},
	})
	got := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(b.String(), "")
	want := "   NAME     BRANCH\n*  current  main\n   other    feature\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}
