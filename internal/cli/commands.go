package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/mklinovsky/worq/internal/config"
	"github.com/mklinovsky/worq/internal/git"
	"github.com/mklinovsky/worq/internal/setup"
	"github.com/mklinovsky/worq/internal/tui"
	"github.com/mklinovsky/worq/internal/worktree"
)

// progress prints setup steps to stderr, so stdout stays parseable.
func progress(quiet bool) setup.Reporter {
	return func(ev setup.Event) {
		if quiet {
			return
		}
		switch {
		case ev.Line != "":
			fmt.Fprintln(os.Stderr, "    "+sDim.Render(ev.Line))
		case ev.Done && ev.Err != nil:
			fmt.Fprintln(os.Stderr, "  "+sErr.Render("✗ "+ev.Label)+": "+ev.Err.Error())
		case ev.Done:
			fmt.Fprintln(os.Stderr, "  "+sOK.Render("✓ ")+ev.Label)
		default:
			fmt.Fprintln(os.Stderr, "  "+sDim.Render("• "+ev.Label))
		}
	}
}

func cmdNew(args []string) error {
	fs := newFlagSet("new")
	base := fs.String("base", "", "branch to start from (default: project base_branch)")
	noSetup := fs.Bool("no-setup", false, "skip the configured setup steps")
	noFetch := fs.Bool("no-fetch", false, "do not fetch the base branch first")
	track := fs.Bool("track", false, "check out an existing origin branch instead of creating one")
	quiet := fs.Bool("quiet", false, "no progress output on stderr")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		if *track {
			return fmt.Errorf("usage: worq new --track <branch-on-origin>")
		}
		return fmt.Errorf("usage: worq new <jira-key|description|branch>")
	}
	svc, _, err := open()
	if err != nil {
		return err
	}
	if !*quiet {
		fmt.Fprintln(os.Stderr, sCommand.Render("→ ")+svc.Cfg.Name)
	}
	c, err := svc.New(worktree.CreateOptions{
		Args:    fs.Args(),
		Base:    *base,
		NoSetup: *noSetup,
		NoFetch: *noFetch,
		Track:   *track,
		Report:  progress(*quiet),
	})
	if c == nil {
		return err
	}
	if !*quiet {
		fmt.Fprintf(os.Stderr, "\n  %s %s\n", sDim.Render("branch"), sOK.Render(c.Branch))
	}
	// The path is the machine-readable result: stdout, always, on its own.
	fmt.Println(c.Path)
	return err
}

type jsonEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Main    bool   `json:"main"`
	Dirty   bool   `json:"dirty"`
	Ahead   int    `json:"ahead"`
	Current bool   `json:"current"`
}

func cmdList(args []string) error {
	fs := newFlagSet("ls")
	asJSON := fs.Bool("json", false, "print JSON instead of opening the browser")
	paths := fs.Bool("paths", false, "print paths only")
	plain := fs.Bool("plain", false, "print a table instead of opening the browser")
	all := fs.Bool("all", false, "include the repository checkout itself")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, cwd, err := open()
	if err != nil {
		return err
	}

	// Without a flag asking for text, ls is the browser.
	if !*asJSON && !*paths && !*plain {
		path, err := tui.Run(svc, cwd)
		if err != nil {
			return err
		}
		if path != "" {
			fmt.Println(path)
		}
		return nil
	}

	if *paths {
		entries, err := svc.List(cwd)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Main && !*all {
				continue
			}
			fmt.Println(e.Path)
		}
		return nil
	}

	entries, err := svc.ListFilled(cwd)
	if err != nil {
		return err
	}
	if !*all {
		kept := entries[:0]
		for _, e := range entries {
			if !e.Main {
				kept = append(kept, e)
			}
		}
		entries = kept
	}

	if *asJSON {
		out := make([]jsonEntry, 0, len(entries))
		for _, e := range entries {
			out = append(out, jsonEntry{e.Name(), e.Path, e.Branch, e.Main, e.Dirty, e.Ahead, e.Current})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, sDim.Render("\tNAME\tBRANCH\tSTATE"))
	for _, e := range entries {
		marker := " "
		if e.Current {
			marker = sOK.Render("*")
		}
		var state []string
		if e.Main {
			state = append(state, sDim.Render("repo"))
		}
		if e.Dirty {
			state = append(state, sFlag.Render("dirty"))
		}
		if e.Locked {
			state = append(state, sDim.Render("locked"))
		}
		if e.Prunable {
			state = append(state, sErr.Render("prunable"))
		}
		if !e.Main && e.Branch != "" {
			if e.Ahead == 0 {
				state = append(state, sDim.Render("merged"))
			} else {
				state = append(state, sOK.Render(fmt.Sprintf("+%d", e.Ahead)))
			}
		}
		branch := e.Branch
		if branch == "" {
			branch = "(detached)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", marker, e.Name(), branch, strings.Join(state, " "))
	}
	return w.Flush()
}

func cmdPath(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: worq path <name|branch|issue>")
	}
	svc, _, err := open()
	if err != nil {
		return err
	}
	e, err := svc.Find(args[0])
	if err != nil {
		return err
	}
	fmt.Println(e.Path)
	return nil
}

func cmdSetup(args []string) error {
	fs := newFlagSet("setup")
	quiet := fs.Bool("quiet", false, "no progress output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, cwd, err := open()
	if err != nil {
		return err
	}
	target := cwd
	if fs.NArg() > 0 {
		e, err := svc.Find(fs.Arg(0))
		if err != nil {
			return err
		}
		target = e.Path
	}
	if len(svc.Cfg.Setup) == 0 {
		return fmt.Errorf("no setup steps configured for %s (see %s)", svc.Cfg.Name, config.Path())
	}
	return setup.Failed(svc.Setup(target, progress(*quiet)))
}

func cmdRemove(args []string) error {
	fs := newFlagSet("rm")
	force := fs.Bool("force", false, "remove even with uncommitted changes")
	keepBranch := fs.Bool("keep-branch", false, "keep the local branch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: worq rm <name|branch|issue>")
	}
	svc, _, err := open()
	if err != nil {
		return err
	}
	e, err := svc.Find(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := svc.Remove(e, *force, !*keepBranch); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, sOK.Render("removed ")+e.Name())
	return nil
}

func cmdClean(args []string) error {
	fs := newFlagSet("clean")
	dryRun := fs.Bool("dry-run", false, "only show what would go")
	keepBranch := fs.Bool("keep-branch", false, "keep the local branches")
	yes := fs.Bool("yes", false, "do not ask")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, _, err := open()
	if err != nil {
		return err
	}
	stale, err := svc.Stale()
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		fmt.Fprintln(os.Stderr, "nothing to clean")
		return nil
	}
	fmt.Fprintf(os.Stderr, "%d %s fully merged into %s:\n", len(stale),
		plural(len(stale), "worktree", "worktrees"), svc.Cfg.BaseBranch)
	for _, e := range stale {
		fmt.Fprintf(os.Stderr, "  %s  %s\n", e.Name(), sDim.Render("("+e.Branch+")"))
	}
	if *dryRun {
		return nil
	}
	if !*yes {
		fmt.Fprint(os.Stderr, "remove them? [y/N] ")
		var answer string
		fmt.Fscanln(os.Stdin, &answer)
		if !strings.EqualFold(strings.TrimSpace(answer), "y") {
			return nil
		}
	}
	var failed int
	for i := range stale {
		if err := svc.Remove(&stale[i], false, !*keepBranch); err != nil {
			failed++
			fmt.Fprintln(os.Stderr, "  "+sErr.Render("✗ "+stale[i].Name())+": "+err.Error())
			continue
		}
		fmt.Fprintln(os.Stderr, "  "+sOK.Render("✓ ")+stale[i].Name())
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d could not be removed", failed, len(stale))
	}
	return nil
}

func cmdConfig(args []string) error {
	fs := newFlagSet("config")
	edit := fs.Bool("edit", false, "open the config file in your editor")
	path := fs.Bool("path", false, "print the config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfgPath := config.Path()
	if *path {
		fmt.Println(cfgPath)
		return nil
	}
	if *edit {
		editor := os.Getenv("VISUAL")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			return fmt.Errorf("set $EDITOR first, or edit %s", cfgPath)
		}
		return runEditor(editor, cfgPath)
	}
	svc, _, err := open()
	if err != nil {
		return err
	}
	c := svc.Cfg
	label := func(k string) string { return sDim.Render(fmt.Sprintf("%-14s", k)) }
	fmt.Println(label("config file") + cfgPath)
	if !c.Matched {
		fmt.Println(sDim.Render("              (no [[projects]] entry matched — using defaults)"))
	}
	fmt.Println(label("project") + c.Name)
	fmt.Println(label("root") + c.Root)
	fmt.Println(label("worktree base") + c.WorktreeBase)
	fmt.Println(label("base branch") + c.BaseBranch)
	if c.JiraKey != "" {
		fmt.Println(label("jira key") + c.JiraKey)
	}
	if c.BranchPrefix != "" {
		fmt.Println(label("branch prefix") + c.BranchPrefix)
	}
	if len(c.Setup) == 0 {
		fmt.Println(label("setup") + sDim.Render("none"))
		return nil
	}
	fmt.Println(sDim.Render("setup"))
	for i, s := range c.Setup {
		suffix := ""
		if s.Optional {
			suffix = sDim.Render("  (optional)")
		}
		if len(s.Copy) == 0 && s.Run == "" {
			suffix += sErr.Render("  does nothing — no copy or run")
		}
		fmt.Printf("  %s %s%s\n", sDim.Render(fmt.Sprintf("%d.", i+1)), s.Label(), suffix)
	}
	return nil
}

func cmdInit(args []string) error {
	fs := newFlagSet("init")
	force := fs.Bool("force", false, "overwrite an existing config with the template")
	plain := fs.Bool("template", false, "write the commented template instead of opening the form")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p := config.Path()

	if *plain || *force {
		if err := config.WriteTemplate(p, *force); err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("%s already exists (use --force, or run worq init inside a repo)", p)
			}
			return err
		}
		fmt.Fprintln(os.Stderr, sOK.Render("wrote ")+p)
		fmt.Fprintln(os.Stderr, indent("add a [[projects]] entry per repo; see the comments in the file"))
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// The form needs a repository to describe. Outside one, fall back to the
	// template so `worq init` is never a dead end.
	if _, err := git.MainRoot(cwd); err != nil {
		fmt.Fprintln(os.Stderr, "not inside a git repository — writing the template instead")
		if err := config.WriteTemplate(p, false); err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("%s already exists — run worq init inside a repository to add it", p)
			}
			return err
		}
		fmt.Fprintln(os.Stderr, sOK.Render("wrote ")+p)
		return nil
	}
	return tui.RunInit(cwd)
}

func runEditor(editor, path string) error {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	return execShell(sh, editor+" "+quote(path), filepath.Dir(path))
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
