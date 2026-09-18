// Package cli wires the commands together.
package cli

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mklinovsky/worq/internal/worktree"
)

// Version is set at build time: -ldflags "-X .../internal/cli.Version=1.2.3".
var Version = "dev"

type command struct {
	name    string
	summary string
	run     func(args []string) error
}

var commands []command

func register(name, summary string, run func([]string) error) {
	commands = append(commands, command{name, summary, run})
}

func init() {
	register("new", "create a worktree and run setup", cmdNew)
	register("ls", "browse worktrees (--plain, --json, --paths for text)", cmdList)
	register("path", "print the path of a worktree", cmdPath)
	register("setup", "re-run setup steps in a worktree", cmdSetup)
	register("rm", "remove a worktree", cmdRemove)
	register("clean", "remove worktrees whose branch is merged", cmdClean)
	register("config", "show the resolved config for this repo", cmdConfig)
	register("init", "register this repo (interactive form)", cmdInit)
	register("version", "print the version", func([]string) error {
		fmt.Println(sTitle.Render("worq") + " " + Version)
		return nil
	})
}

// Main is the entry point. Returns the process exit code.
func Main(argv []string) int {
	if len(argv) == 0 {
		usage()
		return 0
	}
	switch argv[0] {
	case "-h", "--help", "help":
		usage()
		return 0
	case "-v", "--version":
		fmt.Println(sTitle.Render("worq") + " " + Version)
		return 0
	}
	for _, c := range commands {
		if c.name == argv[0] {
			if err := c.run(argv[1:]); err != nil {
				fail(err)
				return 1
			}
			return 0
		}
	}
	fail(fmt.Errorf("unknown command %q (try: worq help)", argv[0]))
	return 1
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, sErr.Render("worq:")+" "+err.Error())
}

func usage() {
	fmt.Println(sTitle.Render("worq") + " — git worktrees and per-project setup, without the alias pile")
	fmt.Println("\n" + sSection.Render("usage"))
	fmt.Println("  " + sCommand.Render("worq") + " <command> [args]")

	fmt.Println("\n" + sSection.Render("commands"))
	names := make([]command, len(commands))
	copy(names, commands)
	sort.Slice(names, func(i, j int) bool { return names[i].name < names[j].name })
	for _, c := range names {
		fmt.Printf("  %s  %s\n", pad(sCommand.Render(c.name), 10), sDim.Render(c.summary))
	}

	fmt.Println("\n" + sSection.Render("examples"))
	for _, ex := range [][2]string{
		{"worq init", "register the repo you are standing in"},
		{"worq new EFRON-1234 fix login redirect", "create efron-1234-fix-login-redirect"},
		{"worq new 1234 fix login", "same, when jira_key is set for the project"},
		{"worq new feat/mobile/login", "explicit branch name, taken as typed"},
		{"worq new --track EFRON-7926-jquery-removal", "check out an existing branch from origin"},
		{"worq ls", "browse worktrees"},
		{"worq ls --plain", "a table instead of the browser"},
		{"worq clean --dry-run", "show merged worktrees that could go"},
	} {
		fmt.Printf("  %s %s\n", pad(colourCommand(ex[0]), 42), sDim.Render(ex[1]))
	}

	fmt.Println("\n" + sSection.Render("session managers"))
	fmt.Println(sDim.Render("  worq never moves you anywhere: progress goes to stderr, the worktree"))
	fmt.Println(sDim.Render("  path to stdout, so your own tooling decides where it opens."))
	for _, ex := range []string{
		`tmux new-session -s fix -c "$(worq new EFRON-1234 fix login)"`,
		`tmux new-window -c "$(worq path 1234)"`,
		`tmux new-window -c "$(worq ls)"`,
	} {
		fmt.Println("  " + colourCommand(ex))
	}
}

// colourCommand tints the program name and any flags in an example line.
func colourCommand(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		switch {
		case w == "worq", strings.HasSuffix(w, "worq"):
			words[i] = strings.Replace(w, "worq", sCommand.Render("worq"), 1)
		case strings.HasPrefix(w, "--"):
			words[i] = sFlag.Render(w)
		}
	}
	return strings.Join(words, " ")
}

// open resolves the service for the current directory.
func open() (*worktree.Service, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	svc, err := worktree.Open(cwd)
	if err != nil {
		return nil, "", err
	}
	return svc, cwd, nil
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("worq "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
