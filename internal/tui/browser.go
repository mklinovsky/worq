// Package tui contains the Bubble Tea interface.
package tui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mklinovsky/worq/internal/setup"
	"github.com/mklinovsky/worq/internal/worktree"
)

type mode int

const (
	modeList mode = iota
	modeConfirm
	modeCreate
	modeWork
)

type item struct {
	e      worktree.Entry
	marked bool
}

func (i item) FilterValue() string { return i.e.Name() + " " + i.e.Branch }

type delegate struct{ width int }

func (delegate) Height() int                         { return 2 }
func (delegate) Spacing() int                        { return 1 }
func (delegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d delegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(item)
	if !ok {
		return
	}
	e := it.e
	selected := index == m.Index()

	gutter := "  "
	if selected {
		gutter = selectedStyle.Render("▌ ")
	}
	mark := "  "
	switch {
	case it.marked:
		mark = okStyle.Render("✓ ")
	case selected:
		mark = subtleStyle.Render("· ")
	}

	name := e.Name()
	switch {
	case selected:
		name = selectedStyle.Render(name)
	case it.marked:
		name = okStyle.Render(name)
	}

	// Everything but the name lives on the dim second line, so nothing is
	// pinned to the right edge of a wide terminal.
	// The branch is only worth a line of its own when it differs from the
	// directory name — and the issue key is already the head of it.
	var parts []string
	switch {
	case e.Branch == "":
		parts = append(parts, subtleStyle.Render("detached at "+short(e.Head)))
	case e.Branch != e.Name():
		parts = append(parts, subtleStyle.Render(e.Branch))
	}
	parts = append(parts, d.status(e)...)
	if e.Current {
		parts = append(parts, okStyle.Render("you are here"))
	}

	line1 := gutter + mark + name
	line2 := "      " + strings.Join(parts, subtleStyle.Render("  ·  "))
	fmt.Fprint(w, line1+"\n"+line2)
}

// status renders the state fragments of the second line.
func (d delegate) status(e worktree.Entry) []string {
	if !e.Loaded {
		return []string{subtleStyle.Render("…")}
	}
	var out []string
	if e.Dirty {
		out = append(out, warnStyle.Render("dirty"))
	}
	if e.Locked {
		out = append(out, badgeStyle.Render("locked"))
	}
	if e.Prunable {
		out = append(out, errStyle.Render("prunable"))
	}
	switch {
	case e.Branch == "":
	case e.Ahead == 0:
		out = append(out, subtleStyle.Render("merged"))
	default:
		out = append(out, okStyle.Render(fmt.Sprintf("+%d", e.Ahead)))
	}
	if e.Age != "" {
		out = append(out, subtleStyle.Render(e.Age))
	}
	return out
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// Model is the worktree browser.
type Model struct {
	svc     *worktree.Service
	cwd     string
	list    list.Model
	mode    mode
	spin    spinner.Model
	input   textinput.Model
	confirm []worktree.Entry
	marked  map[string]bool
	main    *worktree.Entry // the repository checkout: a header, not a row
	loading int
	log     []string
	workMsg string
	status  string
	err     error
	chosen  string // path printed on stdout when the program exits
	width   int
	height  int
}

// New builds the browser model.
func New(svc *worktree.Service, cwd string) Model {
	l := list.New(nil, delegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = "EFRON-123 fix the login redirect"
	ti.CharLimit = 200

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colAccent)

	return Model{svc: svc, cwd: cwd, list: l, spin: sp, input: ti, marked: map[string]bool{}}
}

// Chosen is the path the user selected, if any.
func (m Model) Chosen() string { return m.chosen }

func (m Model) Init() tea.Cmd { return tea.Batch(m.reload(), m.spin.Tick) }

/* ---------- messages ---------- */

type entriesMsg struct {
	entries []worktree.Entry
	err     error
}
type statusMsg struct{ entry worktree.Entry }
type workLineMsg string
type workDoneMsg struct {
	summary string
	path    string
	err     error
}

func (m Model) reload() tea.Cmd {
	svc, cwd := m.svc, m.cwd
	return func() tea.Msg {
		e, err := svc.List(cwd)
		return entriesMsg{entries: e, err: err}
	}
}

// fill computes one worktree's status off the main loop, so thirteen slow
// repositories are thirteen concurrent git calls and not a serial wait.
func (m Model) fill(e worktree.Entry) tea.Cmd {
	svc := m.svc
	return func() tea.Msg { return statusMsg{entry: svc.Fill(e)} }
}

// lines is the channel setup steps report through; a single global is enough
// because only one job runs at a time.
var lines = make(chan tea.Msg, 256)

func waitForLine() tea.Cmd {
	return func() tea.Msg { return <-lines }
}

func (m Model) createCmd(text string) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		args := strings.Fields(text)
		c, err := svc.New(worktree.CreateOptions{
			Args:   args,
			Report: reporter(),
		})
		if err != nil {
			return workDoneMsg{err: err}
		}
		return workDoneMsg{summary: "created " + c.Branch, path: c.Path}
	}
}

func (m Model) setupCmd(e worktree.Entry) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		res := svc.Setup(e.Path, reporter())
		if err := setup.Failed(res); err != nil {
			return workDoneMsg{err: err}
		}
		return workDoneMsg{summary: "setup finished for " + e.Name(), path: e.Path}
	}
}

func (m Model) removeCmd(entries []worktree.Entry, force bool) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		var removed, failed int
		var firstErr error
		for i := range entries {
			e := entries[i]
			lines <- workLineMsg(subtleStyle.Render("• " + e.Name()))
			if err := svc.Remove(&e, force, true); err != nil {
				failed++
				if firstErr == nil {
					firstErr = err
				}
				lines <- workLineMsg(errStyle.Render("✗ "+e.Name()) + " " + err.Error())
				continue
			}
			removed++
			lines <- workLineMsg(okStyle.Render("✓ " + e.Name()))
		}
		switch {
		case failed > 0 && removed == 0:
			return workDoneMsg{err: firstErr}
		case failed > 0:
			return workDoneMsg{summary: fmt.Sprintf("removed %d, %d failed", removed, failed)}
		default:
			return workDoneMsg{summary: fmt.Sprintf("removed %d %s", removed, plural(removed))}
		}
	}
}

func plural(n int) string {
	if n == 1 {
		return "worktree"
	}
	return "worktrees"
}

func reporter() setup.Reporter {
	return func(ev setup.Event) {
		switch {
		case ev.Line != "":
			lines <- workLineMsg("  " + ev.Line)
		case ev.Done && ev.Err != nil:
			lines <- workLineMsg(errStyle.Render("✗ "+ev.Label) + " " + ev.Err.Error())
		case ev.Done:
			lines <- workLineMsg(okStyle.Render("✓ " + ev.Label))
		default:
			lines <- workLineMsg(subtleStyle.Render("• " + ev.Label))
		}
	}
}

/* ---------- update ---------- */

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width-2, max(3, msg.Height-8))
		m.list.SetDelegate(delegate{width: msg.Width - 4})
		m.input.Width = msg.Width - 6
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case entriesMsg:
		m.err = msg.err
		items := make([]list.Item, 0, len(msg.entries))
		live := map[string]bool{}
		cmds := []tea.Cmd{}
		m.main = nil
		for _, e := range msg.entries {
			if e.Main {
				main := e
				m.main = &main
				continue // the repository checkout is a header, not a row
			}
			items = append(items, item{e: e, marked: m.marked[e.Path]})
			live[e.Path] = true
			cmds = append(cmds, m.fill(e))
		}
		for p := range m.marked {
			if !live[p] {
				delete(m.marked, p)
			}
		}
		m.loading = len(cmds)
		cmds = append(cmds, m.list.SetItems(items))
		return m, tea.Batch(cmds...)

	case statusMsg:
		items := m.list.Items()
		for i, li := range items {
			if it, ok := li.(item); ok && it.e.Path == msg.entry.Path {
				it.e = msg.entry
				it.marked = m.marked[msg.entry.Path]
				items[i] = it
			}
		}
		if m.loading > 0 {
			m.loading--
		}
		return m, m.list.SetItems(items)

	case workLineMsg:
		m.log = append(m.log, string(msg))
		if len(m.log) > 200 {
			m.log = m.log[len(m.log)-200:]
		}
		return m, waitForLine()

	case workDoneMsg:
		m.mode = modeList
		if msg.err != nil {
			m.status = errStyle.Render(msg.err.Error())
		} else {
			m.status = okStyle.Render(msg.summary)
		}
		m.log = nil
		return m, m.reload()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeWork:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil

	case modeConfirm:
		switch msg.String() {
		case "y", "Y", "f", "F":
			force := msg.String() == "f" || msg.String() == "F"
			targets := m.confirm
			if !force {
				targets = nil
				for _, e := range m.confirm {
					if !e.Dirty {
						targets = append(targets, e)
					}
				}
			}
			if len(targets) == 0 {
				m.mode = modeList
				m.status = warnStyle.Render("all of those have uncommitted changes — f to remove anyway")
				return m, nil
			}
			m.marked = map[string]bool{}
			m.mode = modeWork
			m.workMsg = fmt.Sprintf("removing %d %s", len(targets), plural(len(targets)))
			m.log = nil
			return m, tea.Batch(m.removeCmd(targets, force), waitForLine(), m.spin.Tick)
		default:
			m.mode = modeList
			return m, nil
		}

	case modeCreate:
		switch msg.String() {
		case "esc":
			m.mode = modeList
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				m.mode = modeList
				return m, nil
			}
			m.input.SetValue("")
			m.mode = modeWork
			m.workMsg = "creating worktree"
			m.log = nil
			return m, tea.Batch(m.createCmd(text), waitForLine(), m.spin.Tick)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	// modeList
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "enter":
		if it, ok := m.list.SelectedItem().(item); ok {
			m.chosen = it.e.Path
		}
		return m, tea.Quit
	case "n":
		m.mode = modeCreate
		m.status = ""
		m.input.Focus()
		return m, textinput.Blink
	case " ", "tab":
		if it, ok := m.list.SelectedItem().(item); ok {
			if it.e.Main {
				m.status = warnStyle.Render("the main worktree cannot be removed")
				return m, nil
			}
			if m.marked[it.e.Path] {
				delete(m.marked, it.e.Path)
			} else {
				m.marked[it.e.Path] = true
			}
			m.status = ""
			m.list.CursorDown()
			return m, m.refreshMarks()
		}
		return m, nil
	case "a":
		// mark everything worq is willing to remove
		for _, li := range m.list.Items() {
			if it, ok := li.(item); ok && !it.e.Main {
				m.marked[it.e.Path] = true
			}
		}
		return m, m.refreshMarks()
	case "A", "ctrl+a":
		m.marked = map[string]bool{}
		return m, m.refreshMarks()
	case "d", "x":
		targets := m.selection()
		if len(targets) == 0 {
			m.status = warnStyle.Render("nothing to remove")
			return m, nil
		}
		m.confirm = targets
		m.mode = modeConfirm
		return m, nil
	case "s":
		if it, ok := m.list.SelectedItem().(item); ok {
			m.mode = modeWork
			m.workMsg = "running setup in " + it.e.Name()
			m.log = nil
			return m, tea.Batch(m.setupCmd(it.e), waitForLine(), m.spin.Tick)
		}
		return m, nil
	case "r":
		m.status = ""
		return m, m.reload()
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// selection returns the marked worktrees, or the one under the cursor when
// nothing is marked.
func (m Model) selection() []worktree.Entry {
	var out []worktree.Entry
	if len(m.marked) > 0 {
		for _, li := range m.list.Items() {
			if it, ok := li.(item); ok && m.marked[it.e.Path] {
				out = append(out, it.e)
			}
		}
		return out
	}
	if it, ok := m.list.SelectedItem().(item); ok && !it.e.Main {
		out = append(out, it.e)
	}
	return out
}

// refreshMarks re-renders the rows with the current marks.
func (m Model) refreshMarks() tea.Cmd {
	items := m.list.Items()
	for i, li := range items {
		if it, ok := li.(item); ok {
			it.marked = m.marked[it.e.Path]
			items[i] = it
		}
	}
	return m.list.SetItems(items)
}

/* ---------- view ---------- */

// header names the project, says where you are standing and where worktrees
// live. The repository checkout is reported here rather than as a list row,
// because it is not something you create, remove or switch between.
func (m Model) header() string {
	left := titleStyle.Render(m.svc.Cfg.Name)
	if m.main != nil {
		branch := m.main.Branch
		if branch == "" {
			branch = short(m.main.Head)
		}
		left += subtleStyle.Render("  on ") + badgeStyle.Render(branch)
		if m.main.Current {
			left += okStyle.Render("  ← you are here")
		}
	}
	right := ""
	if n := len(m.list.Items()); n > 0 {
		right = subtleStyle.Render(fmt.Sprintf("%d %s", n, plural(n)))
		if m.loading > 0 {
			right = subtleStyle.Render(fmt.Sprintf("%d %s · loading", n, plural(n)))
		}
	}
	pad := m.chromeWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	var b strings.Builder
	b.WriteString(headerStyle.Render(left+strings.Repeat(" ", pad)+right) + "\n")
	b.WriteString(helpStyle.Render(m.svc.Cfg.WorktreeBase) + "\n")
	b.WriteString(ruleStyle.Render(strings.Repeat("─", m.chromeWidth())) + "\n\n")
	return b.String()
}

// chromeWidth caps the header and rules so they stay readable on a wide
// monitor instead of stretching to the edge of the screen.
func (m Model) chromeWidth() int {
	w := m.width - 2
	if w > 110 {
		w = 110
	}
	if w < 10 {
		w = 10
	}
	return w
}

func (m Model) footer(help string) string {
	var b strings.Builder
	b.WriteString("\n" + ruleStyle.Render(strings.Repeat("─", m.chromeWidth())) + "\n")
	b.WriteString(helpStyle.Render(help))
	return b.String()
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(m.header())

	switch m.mode {
	case modeWork:
		b.WriteString(bodyStyle.Render(m.spin.View()+" "+m.workMsg) + "\n\n")
		for _, l := range tail(m.log, max(3, m.height-8)) {
			b.WriteString(bodyStyle.Render(l) + "\n")
		}
		return b.String()

	case modeCreate:
		b.WriteString(bodyStyle.Render("New worktree — Jira key or a description:") + "\n")
		b.WriteString(bodyStyle.Render(m.input.View()) + "\n\n")
		b.WriteString(m.footer("enter create · esc cancel"))
		return b.String()

	case modeConfirm:
		dirty := 0
		b.WriteString(bodyStyle.Render(fmt.Sprintf("Remove %d %s and their branches?",
			len(m.confirm), plural(len(m.confirm)))) + "\n\n")
		for _, e := range m.confirm {
			line := "  " + e.Name() + subtleStyle.Render("  "+e.Branch)
			if e.Dirty {
				dirty++
				line += warnStyle.Render("  uncommitted changes")
			}
			b.WriteString(bodyStyle.Render(line) + "\n")
		}
		b.WriteString("\n")
		if dirty > 0 {
			b.WriteString(bodyStyle.Render(warnStyle.Render(
				fmt.Sprintf("%d of those would lose work — y skips them, f removes them anyway", dirty))) + "\n\n")
			b.WriteString(m.footer("y remove the clean ones · f remove all · any other key cancel"))
		} else {
			b.WriteString(m.footer("y remove · any other key cancel"))
		}
		return b.String()
	}

	if m.err != nil {
		b.WriteString(bodyStyle.Render(errStyle.Render(m.err.Error())) + "\n")
	}
	if len(m.list.Items()) == 0 {
		b.WriteString(bodyStyle.Render(subtleStyle.Render("no worktrees yet — press n to create one")) + "\n")
	} else {
		b.WriteString(m.list.View())
	}
	if m.status != "" {
		b.WriteString("\n" + bodyStyle.Render(m.status))
	}
	help := "enter path · n new · s setup · space mark · d remove · / filter · q quit"
	if n := len(m.marked); n > 0 {
		help = fmt.Sprintf("%d marked · d remove · space unmark · a all · A none · q quit", n)
	}
	b.WriteString(m.footer(help))
	return b.String()
}

func tail(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Run starts the browser. The UI is drawn on stderr so the chosen path can be
// printed on stdout for shell integration.
func Run(svc *worktree.Service, cwd string) (string, error) {
	p := tea.NewProgram(New(svc, cwd), tea.WithOutput(os.Stderr), tea.WithAltScreen())
	res, err := p.Run()
	if err != nil {
		return "", err
	}
	if m, ok := res.(Model); ok {
		return m.Chosen(), nil
	}
	return "", nil
}
