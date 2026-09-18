package tui

import "github.com/charmbracelet/lipgloss"

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#3b5bdb", Dark: "#8ea5ff"}
	colMuted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#8b94a6"}
	colWarn   = lipgloss.AdaptiveColor{Light: "#b45309", Dark: "#f0b429"}
	colOK     = lipgloss.AdaptiveColor{Light: "#1a7f5a", Dark: "#5ad19b"}
	colErr    = lipgloss.AdaptiveColor{Light: "#b42318", Dark: "#ff8a80"}
	colRule   = lipgloss.AdaptiveColor{Light: "#d7dae0", Dark: "#3a3f4b"}

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	subtleStyle   = lipgloss.NewStyle().Foreground(colMuted)
	warnStyle     = lipgloss.NewStyle().Foreground(colWarn)
	okStyle       = lipgloss.NewStyle().Foreground(colOK)
	errStyle      = lipgloss.NewStyle().Foreground(colErr)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	headerStyle   = lipgloss.NewStyle().Padding(0, 1)
	bodyStyle     = lipgloss.NewStyle().Padding(0, 1)
	helpStyle     = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	badgeStyle    = lipgloss.NewStyle().Foreground(colMuted)
	ruleStyle     = lipgloss.NewStyle().Foreground(colRule).Padding(0, 1)
)
