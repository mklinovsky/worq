package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mklinovsky/worq/internal/config"
	"github.com/mklinovsky/worq/internal/detect"
)

type initMode int

const (
	initForm initMode = iota
	initStepEdit
	initSaved
)

// field indices in the focus ring; stepsFocus sits at the end of it.
const (
	fName = iota
	fPath
	fWorktreeBase
	fBaseBranch
	fJiraKey
	fBranchPrefix
	numFields

	stepsFocus = numFields // the steps list sits at the end of the focus ring
	ringLen    = numFields + 1
)

var fieldLabels = [numFields]string{
	fName:         "name",
	fPath:         "path",
	fWorktreeBase: "worktree base",
	fBaseBranch:   "base branch",
	fJiraKey:      "jira key",
	fBranchPrefix: "branch prefix",
}

var fieldHints = [numFields]string{
	fName:         "how worq refers to this project",
	fPath:         "matched against your working directory",
	fWorktreeBase: "{root} {parent} {name} are expanded",
	fBaseBranch:   "new branches start here",
	fJiraKey:      "lets you type: worq new 1234 fix login",
	fBranchPrefix: "prepended when the name has no issue key",
}

type stepKind int

const (
	kindCopy stepKind = iota
	kindRun
)

var stepKinds = []stepKind{kindCopy, kindRun}

func (k stepKind) String() string {
	if k == kindCopy {
		return "copy"
	}
	return "run"
}

func kindOf(s config.Step) stepKind {
	if s.Run != "" {
		return kindRun
	}
	return kindCopy
}

func stepValue(s config.Step) string {
	if kindOf(s) == kindRun {
		return s.Run
	}
	return strings.Join(s.Copy, ", ")
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// InitModel is the `worq init` form.
type InitModel struct {
	cfgPath  string
	cfg      *config.Config
	det      detect.Result
	newFile  bool
	replaced bool

	mode   initMode
	focus  int
	inputs [numFields]textinput.Model
	steps  []config.Step
	sel    int

	// step editor
	edit      config.Step
	editKind  stepKind
	editIdx   int // -1 when adding
	editFocus int
	editIn    [3]textinput.Model // name, value, dir
	editOpt   bool

	status string
	err    error
	width  int
	height int
}

// NewInit builds the init form for the repository containing cwd.
func NewInit(cwd string) (InitModel, error) {
	path := config.Path()
	cfg, err := config.Load()
	if err != nil {
		return InitModel{}, err
	}
	_, statErr := os.Stat(path)
	m := InitModel{cfgPath: path, cfg: cfg, newFile: os.IsNotExist(statErr)}

	det, err := detect.Inspect(cwd, cfg)
	if err != nil {
		return InitModel{}, err
	}
	m.det = det

	values := [numFields]string{
		fName:         det.Name,
		fPath:         det.Path,
		fWorktreeBase: det.WorktreeBase,
		fBaseBranch:   det.BaseBranch,
		fJiraKey:      det.JiraKey,
		fBranchPrefix: "",
	}
	m.steps = det.Suggestions

	// An already registered project is edited rather than duplicated.
	if p := det.Existing; p != nil {
		m.replaced = true
		values[fName] = or(p.Name, det.Name)
		values[fPath] = p.Path
		values[fWorktreeBase] = or(p.WorktreeBase, det.WorktreeBase)
		values[fBaseBranch] = or(p.BaseBranch, det.BaseBranch)
		values[fJiraKey] = or(p.JiraKey, det.JiraKey)
		values[fBranchPrefix] = p.BranchPrefix
		if len(p.Setup) > 0 {
			m.steps = p.Setup
		}
	}

	for i := range m.inputs {
		in := textinput.New()
		in.Prompt = ""
		in.CharLimit = 300
		in.SetValue(values[i])
		m.inputs[i] = in
	}
	m.inputs[fJiraKey].Placeholder = "none"
	m.inputs[fBranchPrefix].Placeholder = "none"
	m.focus = fName
	m.inputs[fName].Focus()

	for i := range m.editIn {
		in := textinput.New()
		in.Prompt = ""
		in.CharLimit = 300
		m.editIn[i] = in
	}
	return m, nil
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (m InitModel) Init() tea.Cmd { return textinput.Blink }

/* ---------- update ---------- */

func (m InitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Leave room for the hint that trails the focused field.
		w := max(20, min(52, msg.Width-46))
		for i := range m.inputs {
			m.inputs[i].Width = w
		}
		for i := range m.editIn {
			m.editIn[i].Width = max(24, min(60, msg.Width-24))
		}
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case initSaved:
			return m, tea.Quit
		case initStepEdit:
			return m.updateStepEdit(msg)
		default:
			return m.updateForm(msg)
		}
	}
	return m, nil
}

func (m InitModel) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m, tea.Quit
	// ctrl+s is XOFF in terminals that still have flow control on, so accept
	// a couple of spellings — and enter, from any of the text fields.
	case "ctrl+s", "ctrl+w":
		return m.save()
	case "enter":
		if m.focus < len(m.inputs) {
			return m.save()
		}
	case "tab", "down":
		return m.moveFocus(1), textinput.Blink
	case "shift+tab", "up":
		if m.focus == stepsFocus && m.sel > 0 {
			m.sel--
			return m, nil
		}
		return m.moveFocus(-1), textinput.Blink
	}

	if m.focus == stepsFocus {
		return m.updateStepsList(msg)
	}

	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

// moveFocus walks the ring, and inside the steps section walks the steps
// first so tab feels like one continuous list.
func (m InitModel) moveFocus(d int) InitModel {
	if m.focus == stepsFocus && d > 0 && m.sel < len(m.steps)-1 {
		m.sel++
		return m
	}
	if m.focus < len(m.inputs) {
		m.inputs[m.focus].Blur()
	}
	m.focus = (m.focus + d + ringLen) % ringLen
	if m.focus < len(m.inputs) {
		m.inputs[m.focus].Focus()
	} else {
		if d > 0 {
			m.sel = 0
		} else {
			m.sel = max(0, len(m.steps)-1)
		}
	}
	return m
}

func (m InitModel) updateStepsList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "a":
		m.editIdx = -1
		m.edit = config.Step{}
		m.editKind = kindCopy
		m.editOpt = false
		return m.enterStepEdit(), textinput.Blink
	case "e", "enter":
		if len(m.steps) == 0 {
			return m, nil
		}
		m.editIdx = m.sel
		m.edit = m.steps[m.sel]
		m.editKind = kindOf(m.edit)
		m.editOpt = m.edit.Optional
		return m.enterStepEdit(), textinput.Blink
	case "d", "x":
		if len(m.steps) > 0 {
			m.steps = append(m.steps[:m.sel], m.steps[m.sel+1:]...)
			m.sel = max(0, min(m.sel, len(m.steps)-1))
		}
		return m, nil
	case " ":
		if len(m.steps) > 0 {
			m.steps[m.sel].Optional = !m.steps[m.sel].Optional
		}
		return m, nil
	case "J":
		if m.sel < len(m.steps)-1 {
			m.steps[m.sel], m.steps[m.sel+1] = m.steps[m.sel+1], m.steps[m.sel]
			m.sel++
		}
		return m, nil
	case "K":
		if m.sel > 0 {
			m.steps[m.sel], m.steps[m.sel-1] = m.steps[m.sel-1], m.steps[m.sel]
			m.sel--
		}
		return m, nil
	case "r":
		m.steps = m.det.Suggestions
		m.sel = 0
		m.status = "restored the detected steps"
		return m, nil
	}
	return m, nil
}

func (m InitModel) enterStepEdit() InitModel {
	m.mode = initStepEdit
	m.editFocus = 1 // start on the name, the kind is picked with ←/→
	m.editIn[0].SetValue(m.edit.Name)
	m.editIn[1].SetValue(stepValue(m.edit))
	m.editIn[2].SetValue(m.edit.Dir)
	for i := range m.editIn {
		m.editIn[i].Blur()
	}
	m.editIn[0].Focus()
	return m
}

func (m InitModel) updateStepEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	editFields := 4 // kind, name, value, optional
	if m.editKind == kindRun {
		editFields = 5 // + dir
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = initForm
		return m, nil
	case "tab", "down":
		m.editFocus = (m.editFocus + 1) % editFields
	case "shift+tab", "up":
		m.editFocus = (m.editFocus - 1 + editFields) % editFields
	case "enter":
		return m.commitStep(), nil
	case "left", "right":
		switch m.editFocus {
		case 0:
			d := 1
			if msg.String() == "left" {
				d = -1
			}
			m.editKind = stepKind((int(m.editKind) + d + len(stepKinds)) % len(stepKinds))
		case editFields - 1:
			m.editOpt = !m.editOpt
		}
	case " ":
		if m.editFocus == editFields-1 {
			m.editOpt = !m.editOpt
			return m, nil
		}
	}

	// route typing to the focused input
	idx := -1
	switch m.editFocus {
	case 1:
		idx = 0
	case 2:
		idx = 1
	case 3:
		if m.editKind == kindRun {
			idx = 2
		}
	}
	for i := range m.editIn {
		if i == idx {
			m.editIn[i].Focus()
		} else {
			m.editIn[i].Blur()
		}
	}
	if idx >= 0 {
		var cmd tea.Cmd
		m.editIn[idx], cmd = m.editIn[idx].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m InitModel) commitStep() InitModel {
	s := config.Step{
		Name:     strings.TrimSpace(m.editIn[0].Value()),
		Optional: m.editOpt,
	}
	value := strings.TrimSpace(m.editIn[1].Value())
	if m.editKind == kindRun {
		s.Run = value
		s.Dir = strings.TrimSpace(m.editIn[2].Value())
	} else {
		s.Copy = splitList(value)
	}
	if stepValue(s) == "" {
		m.mode = initForm
		m.status = warnStyle.Render("step needs a value — nothing added")
		return m
	}
	if m.editIdx < 0 {
		m.steps = append(m.steps, s)
		m.sel = len(m.steps) - 1
	} else {
		m.steps[m.editIdx] = s
	}
	m.mode = initForm
	m.status = ""
	return m
}

// project builds the entry from the current form state.
func (m InitModel) project() config.Project {
	p := config.Project{
		Name:         strings.TrimSpace(m.inputs[fName].Value()),
		Path:         strings.TrimSpace(m.inputs[fPath].Value()),
		WorktreeBase: strings.TrimSpace(m.inputs[fWorktreeBase].Value()),
		BaseBranch:   strings.TrimSpace(m.inputs[fBaseBranch].Value()),
		JiraKey:      strings.TrimSpace(m.inputs[fJiraKey].Value()),
		BranchPrefix: strings.TrimSpace(m.inputs[fBranchPrefix].Value()),
		Setup:        m.steps,
	}
	return p
}

func (m InitModel) save() (tea.Model, tea.Cmd) {
	p := m.project()
	if p.Path == "" {
		m.status = errStyle.Render("path cannot be empty")
		return m, nil
	}
	// A brand new file gets a defaults block worth inheriting.
	if m.newFile && m.cfg.Defaults.WorktreeBase == "" {
		m.cfg.Defaults.WorktreeBase = "{parent}/{name}-worktrees"
		m.cfg.Defaults.BaseBranch = "main"
	}
	m.replaced = m.cfg.Upsert(p)
	if err := m.cfg.Save(m.cfgPath); err != nil {
		m.err = err
		m.status = errStyle.Render(err.Error())
		return m, nil
	}
	m.mode = initSaved
	return m, nil
}

/* ---------- view ---------- */

func (m InitModel) View() string {
	if m.mode == initSaved {
		return m.savedView()
	}
	if m.mode == initStepEdit {
		return m.stepEditView()
	}

	var b strings.Builder
	title := "worq init — " + m.det.Name
	if m.replaced {
		title += subtleStyle.Render("  (already registered, editing)")
	}
	b.WriteString(headerStyle.Render(titleStyle.Render(title)) + "\n")
	b.WriteString(helpStyle.Render(m.cfgPath) + "\n\n")

	for i := range fieldLabels {
		b.WriteString(m.fieldRow(i) + "\n")
	}

	b.WriteString("\n" + m.stepsSection())

	if m.status != "" {
		b.WriteString("\n" + bodyStyle.Render(m.status) + "\n")
	}
	b.WriteString("\n" + m.previewSection())
	b.WriteString("\n" + helpStyle.Render(m.helpLine()))
	return b.String()
}

func (m InitModel) fieldRow(i int) string {
	cursor := "  "
	label := fmt.Sprintf("%-14s", fieldLabels[i])
	if m.focus == i {
		cursor = selectedStyle.Render("▸ ")
		label = selectedStyle.Render(label)
	} else {
		label = subtleStyle.Render(label)
	}
	row := cursor + label + m.inputs[i].View()
	if m.focus == i {
		row += subtleStyle.Render("   " + fieldHints[i])
	}
	return bodyStyle.Render(row)
}

func (m InitModel) stepsSection() string {
	var b strings.Builder
	head := "  setup steps"
	if m.focus == stepsFocus {
		head = selectedStyle.Render("▸ setup steps")
	} else {
		head = subtleStyle.Render(head)
	}
	b.WriteString(bodyStyle.Render(head) + "\n")

	if len(m.steps) == 0 {
		b.WriteString(bodyStyle.Render(subtleStyle.Render("      none — tab here and press a to add one")) + "\n")
		return b.String()
	}
	for i, s := range m.steps {
		marker := "    "
		if m.focus == stepsFocus && i == m.sel {
			marker = selectedStyle.Render("  ▸ ")
		}
		kind := map[stepKind]string{kindCopy: "copy", kindRun: "run "}[kindOf(s)]
		line := fmt.Sprintf("%s%-20s %s %s", marker, truncate(s.Name, 20),
			badgeStyle.Render(kind), truncate(stepValue(s), max(20, m.width-46)))
		if s.Optional {
			line += subtleStyle.Render("  optional")
		}
		b.WriteString(bodyStyle.Render(line) + "\n")
	}
	return b.String()
}

func (m InitModel) previewSection() string {
	preview := config.MarshalProject(m.project())
	lines := strings.Split(strings.TrimSpace(preview), "\n")
	budget := m.height - (len(fieldLabels) + len(m.steps) + 11)
	if budget < 3 {
		budget = 3
	}
	if len(lines) > budget {
		lines = append(lines[:budget], subtleStyle.Render(fmt.Sprintf("… %d more lines", len(lines)-budget)))
	}
	var b strings.Builder
	b.WriteString(bodyStyle.Render(subtleStyle.Render("what will be written")) + "\n")
	for _, l := range lines {
		b.WriteString(bodyStyle.Render(subtleStyle.Render("  "+l)) + "\n")
	}
	return b.String()
}

func (m InitModel) helpLine() string {
	if m.focus == stepsFocus {
		return "a add · e edit · d delete · space optional · J/K reorder · r reset · ctrl+s save · esc quit"
	}
	return "tab/↑↓ move · enter or ctrl+s save · esc quit"
}

func (m InitModel) stepEditView() string {
	var b strings.Builder
	verb := "edit step"
	if m.editIdx < 0 {
		verb = "new step"
	}
	b.WriteString(headerStyle.Render(titleStyle.Render("worq init — "+verb)) + "\n\n")

	kinds := []string{"copy", "run"}
	var rendered []string
	for i, k := range kinds {
		if stepKind(i) == m.editKind {
			rendered = append(rendered, selectedStyle.Render("("+k+")"))
		} else {
			rendered = append(rendered, subtleStyle.Render(" "+k+" "))
		}
	}
	b.WriteString(editRow(m.editFocus == 0, "kind", strings.Join(rendered, " ")) + "\n")
	b.WriteString(editRow(m.editFocus == 1, "name", m.editIn[0].View()) + "\n")
	b.WriteString(editRow(m.editFocus == 2, valueLabel(m.editKind), m.editIn[1].View()) + "\n")
	last := 3
	if m.editKind == kindRun {
		b.WriteString(editRow(m.editFocus == 3, "in directory", m.editIn[2].View()) + "\n")
		last = 4
	}
	box := "[ ]"
	if m.editOpt {
		box = okStyle.Render("[x]")
	}
	b.WriteString(editRow(m.editFocus == last, "optional", box) + "\n")

	hint := map[stepKind]string{
		kindCopy: "comma separated paths, copied from the main worktree",
		kindRun:  "shell command, run in the new worktree",
	}[m.editKind]
	b.WriteString("\n" + bodyStyle.Render(subtleStyle.Render(hint)) + "\n")
	b.WriteString("\n" + helpStyle.Render("←/→ change kind · tab move · enter accept · esc cancel"))
	return b.String()
}

func valueLabel(k stepKind) string {
	if k == kindCopy {
		return "copy paths"
	}
	return "command"
}

func editRow(focused bool, label, value string) string {
	cursor := "  "
	l := fmt.Sprintf("%-14s", label)
	if focused {
		cursor = selectedStyle.Render("▸ ")
		l = selectedStyle.Render(l)
	} else {
		l = subtleStyle.Render(l)
	}
	return bodyStyle.Render(cursor + l + value)
}

func (m InitModel) savedView() string {
	var b strings.Builder
	verb := "added"
	if m.replaced {
		verb = "updated"
	}
	b.WriteString(headerStyle.Render(okStyle.Render("✓ "+verb+" "+m.project().Name)) + "\n\n")
	b.WriteString(bodyStyle.Render(m.cfgPath) + "\n\n")
	b.WriteString(bodyStyle.Render(subtleStyle.Render("worq config      see what resolves here")) + "\n")
	b.WriteString(bodyStyle.Render(subtleStyle.Render("worq new …       create a worktree")) + "\n\n")
	b.WriteString(helpStyle.Render("any key to exit"))
	return b.String()
}

func truncate(s string, n int) string {
	if n < 4 || len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RunInit starts the init form for the repository containing cwd.
func RunInit(cwd string) error {
	m, err := NewInit(cwd)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
