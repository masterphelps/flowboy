// internal/tui/style.go
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Pip-Boy color palette (from CodePen reference - Fallout 4 Pip-Boy CSS by Stix)
	colorGreen    = lipgloss.Color("#8df776")
	colorDimGreen = lipgloss.Color("#172f18")
	colorBlack    = lipgloss.Color("#000000")
	colorPanel    = lipgloss.Color("#272b2a")
	colorAccent   = lipgloss.Color("#d8c99e")
	colorBorder   = lipgloss.Color("#333333")
	colorBright   = lipgloss.Color("#7ff12a")

	// Panel styles
	panelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorGreen).
			Background(colorBlack).
			Foreground(colorGreen).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Transform(strings.ToUpper).
			MarginBottom(1)

	activeItemStyle = lipgloss.NewStyle().
			Foreground(colorBright).
			Bold(true)

	dimItemStyle = lipgloss.NewStyle().
			Foreground(colorDimGreen)

	statusBarStyle = lipgloss.NewStyle().
			Background(colorPanel).
			Foreground(colorGreen).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Transform(strings.ToUpper).
			Align(lipgloss.Center).
			MarginBottom(1)
)
