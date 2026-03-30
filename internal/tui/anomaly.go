package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/masterphelps/flowboy/internal/anomaly"
)

type anomalyMode int

const (
	anomalyPicking anomalyMode = iota
	anomalyTweaking
)

// AnomalyStartMsg is emitted when the user confirms an anomaly.
type AnomalyStartMsg struct {
	Scenario  anomaly.Scenario
	Duration  time.Duration
	Intensity float64
	Targets   []string
	Count     int
}

// AnomalyClearMsg is emitted when the user requests clearing all anomalies.
type AnomalyClearMsg struct{}

// AnomalyPanel handles the anomaly scenario picker and tweak form.
type AnomalyPanel struct {
	scenarios      []anomaly.Scenario
	cursor         int
	mode           anomalyMode
	width          int
	height         int
	durationInput  textinput.Model
	intensityInput textinput.Model
	targetsInput   textinput.Model
	countInput     textinput.Model
	formFocus      int
	selected       anomaly.Scenario
}

// NewAnomalyPanel creates an anomaly panel with all 9 scenarios.
func NewAnomalyPanel() AnomalyPanel {
	di := textinput.New()
	di.Placeholder = "60s"
	di.CharLimit = 10
	di.Width = 10

	ii := textinput.New()
	ii.Placeholder = "5.0"
	ii.CharLimit = 6
	ii.Width = 8

	ti := textinput.New()
	ti.Placeholder = "all (comma-separated)"
	ti.CharLimit = 80
	ti.Width = 30

	ci := textinput.New()
	ci.Placeholder = "50"
	ci.CharLimit = 5
	ci.Width = 6

	return AnomalyPanel{
		scenarios:      anomaly.AllScenarios(),
		durationInput:  di,
		intensityInput: ii,
		targetsInput:   ti,
		countInput:     ci,
	}
}

// SetSize updates the available dimensions.
func (p *AnomalyPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// Update handles key messages.
func (p *AnomalyPanel) Update(msg tea.KeyMsg) tea.Cmd {
	switch p.mode {
	case anomalyPicking:
		return p.updatePicking(msg)
	case anomalyTweaking:
		return p.updateTweaking(msg)
	}
	return nil
}

func (p *AnomalyPanel) updatePicking(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		if p.cursor < len(p.scenarios)-1 {
			p.cursor++
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "enter":
		p.enterTweakMode()
	}
	return nil
}

func (p *AnomalyPanel) enterTweakMode() {
	s := p.scenarios[p.cursor]
	p.selected = s
	p.mode = anomalyTweaking
	p.formFocus = 0
	p.durationInput.SetValue(s.DefaultDuration.String())
	p.intensityInput.SetValue(fmt.Sprintf("%.1f", s.DefaultIntensity))
	p.targetsInput.SetValue("")
	p.countInput.SetValue(strconv.Itoa(s.DefaultCount))
	p.durationInput.Focus()
	p.intensityInput.Blur()
	p.targetsInput.Blur()
	p.countInput.Blur()
}

const anomalyFormFieldCount = 4

func (p *AnomalyPanel) updateTweaking(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.mode = anomalyPicking
		return nil
	case "tab", "down":
		p.formFocus = (p.formFocus + 1) % anomalyFormFieldCount
		p.focusField()
		return nil
	case "shift+tab", "up":
		p.formFocus = (p.formFocus + anomalyFormFieldCount - 1) % anomalyFormFieldCount
		p.focusField()
		return nil
	case "enter":
		return p.confirmAnomaly()
	}
	p.updateFocusedInput(msg)
	return nil
}

func (p *AnomalyPanel) focusField() {
	p.durationInput.Blur()
	p.intensityInput.Blur()
	p.targetsInput.Blur()
	p.countInput.Blur()
	switch p.formFocus {
	case 0:
		p.durationInput.Focus()
	case 1:
		p.intensityInput.Focus()
	case 2:
		p.targetsInput.Focus()
	case 3:
		p.countInput.Focus()
	}
}

func (p *AnomalyPanel) updateFocusedInput(msg tea.KeyMsg) {
	switch p.formFocus {
	case 0:
		m, _ := p.durationInput.Update(msg)
		p.durationInput = m
	case 1:
		m, _ := p.intensityInput.Update(msg)
		p.intensityInput = m
	case 2:
		m, _ := p.targetsInput.Update(msg)
		p.targetsInput = m
	case 3:
		m, _ := p.countInput.Update(msg)
		p.countInput = m
	}
}

func (p *AnomalyPanel) confirmAnomaly() tea.Cmd {
	dur, err := time.ParseDuration(strings.TrimSpace(p.durationInput.Value()))
	if err != nil {
		dur = p.selected.DefaultDuration
	}
	intensity, err := strconv.ParseFloat(strings.TrimSpace(p.intensityInput.Value()), 64)
	if err != nil {
		intensity = p.selected.DefaultIntensity
	}
	var targets []string
	raw := strings.TrimSpace(p.targetsInput.Value())
	if raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				targets = append(targets, t)
			}
		}
	}
	count, err := strconv.Atoi(strings.TrimSpace(p.countInput.Value()))
	if err != nil {
		count = p.selected.DefaultCount
	}

	p.mode = anomalyPicking
	msg := AnomalyStartMsg{
		Scenario:  p.selected,
		Duration:  dur,
		Intensity: intensity,
		Targets:   targets,
		Count:     count,
	}
	return func() tea.Msg { return msg }
}

// View renders the anomaly panel.
func (p *AnomalyPanel) View() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("INTRODUCE ANOMALY"))
	b.WriteString("\n")

	switch p.mode {
	case anomalyPicking:
		b.WriteString(p.renderPicker())
	case anomalyTweaking:
		b.WriteString(p.renderTweakForm())
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).
		Render("  Enter: select  Esc: back"))
	return b.String()
}

func (p *AnomalyPanel) renderPicker() string {
	var b strings.Builder
	for i, s := range p.scenarios {
		prefix := "  "
		if i == p.cursor {
			prefix = "\u25b8 "
		}

		var color lipgloss.Color
		switch s.Category {
		case anomaly.CategoryAttack:
			color = colorAnomalyAttack
		case anomaly.CategoryVolume:
			color = colorAnomalyVolume
		case anomaly.CategoryPattern:
			color = colorAnomalyPattern
		}

		nameStyle := lipgloss.NewStyle().Foreground(color).Bold(true)
		descStyle := lipgloss.NewStyle().Foreground(colorAccent)
		line := fmt.Sprintf("%s%-20s  %s", prefix, nameStyle.Render(s.Name), descStyle.Render(s.Description))

		if i == p.cursor {
			b.WriteString(activeItemStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (p *AnomalyPanel) renderTweakForm() string {
	var b strings.Builder
	nameStyle := lipgloss.NewStyle().Foreground(colorAnomalyBanner).Bold(true)
	b.WriteString(nameStyle.Render(p.selected.Name))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render(p.selected.Description))
	b.WriteString("\n\n")

	fields := []struct {
		label string
		view  string
	}{
		{"Duration:  ", p.durationInput.View()},
		{"Intensity: ", p.intensityInput.View()},
		{"Targets:   ", p.targetsInput.View()},
		{"Count:     ", p.countInput.View()},
	}
	for i, f := range fields {
		prefix := "  "
		if i == p.formFocus {
			prefix = "\u25b8 "
		}
		b.WriteString(prefix + f.label + f.view + "\n")
	}
	b.WriteString("\n  Enter: fire  Esc: back  Tab: next field\n")
	return b.String()
}
