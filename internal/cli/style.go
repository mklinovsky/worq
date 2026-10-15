package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Terminal styling for text output. lipgloss degrades to plain text on its own
// when stdout is not a terminal, or when NO_COLOR is set.
var (
	accent = lipgloss.AdaptiveColor{Light: "#3b5bdb", Dark: "#8ea5ff"}
	muted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#8b94a6"}
	danger = lipgloss.AdaptiveColor{Light: "#b42318", Dark: "#ff8a80"}
	good   = lipgloss.AdaptiveColor{Light: "#1a7f5a", Dark: "#5ad19b"}

	sTitle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	sSection = lipgloss.NewStyle().Bold(true)
	sCommand = lipgloss.NewStyle().Foreground(accent)
	sErr     = lipgloss.NewStyle().Foreground(danger)
	sOK      = lipgloss.NewStyle().Foreground(good)
	sFlag    = lipgloss.NewStyle().Foreground(good)

	// Faint as well as coloured: in a 16-colour terminal the muted grey and the
	// accent blue degrade to the same ANSI code, and annotations have to stay
	// visibly dimmer than what they annotate.
	sDim = lipgloss.NewStyle().Faint(true).Foreground(muted)
)

// pad appends spaces to a styled string so columns line up: len() counts the
// escape sequences, lipgloss.Width does not.
func pad(s string, width int) string {
	if n := width - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// table prints rows in columns two spaces apart, sized by visible width.
func table(w io.Writer, rows [][]string) {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], lipgloss.Width(cell))
		}
	}
	for _, row := range rows {
		var b strings.Builder
		for i, cell := range row {
			if i == len(row)-1 {
				b.WriteString(cell)
			} else {
				b.WriteString(pad(cell, widths[i]+2))
			}
		}
		fmt.Fprintln(w, b.String())
	}
}
