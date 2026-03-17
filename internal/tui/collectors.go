// internal/tui/collectors.go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/masterphelps/flowboy/internal/config"
	"github.com/masterphelps/flowboy/internal/engine"
)

// collectorMode describes what the collector panel is currently doing.
type collectorMode int

const (
	collectorNormal   collectorMode = iota
	collectorAdding                 // inline form for new collector
	collectorEditing                // inline form for editing selected collector
	collectorDeleting               // "Are you sure? y/n" prompt
)

// CollectorDisplay holds display-ready information for a single collector row.
type CollectorDisplay struct {
	Name        string
	Address     string
	Version     string
	Connected   bool
	PacketsSent uint64
	BytesSent   uint64
	Errors      uint64
}

// CollectorPanel is a sub-model that renders the collector status bar and
// handles CRUD operations via inline forms.
type CollectorPanel struct {
	collectors   []CollectorDisplay
	cursor       int
	mode         collectorMode
	width        int
	height       int
	nameInput    textinput.Model
	addressInput textinput.Model
	versionInput textinput.Model // "v9" or "ipfix"
	formFocus    int             // 0=name, 1=address, 2=version
	editName     string          // original name when editing
}

// NewCollectorPanel returns an initialised CollectorPanel.
func NewCollectorPanel() CollectorPanel {
	ni := textinput.New()
	ni.Placeholder = "collector-name"
	ni.CharLimit = 40
	ni.Width = 20

	ai := textinput.New()
	ai.Placeholder = "host:port"
	ai.CharLimit = 60
	ai.Width = 30

	vi := textinput.New()
	vi.Placeholder = "v9"
	vi.CharLimit = 5
	vi.Width = 6

	return CollectorPanel{
		nameInput:    ni,
		addressInput: ai,
		versionInput: vi,
	}
}

// SetCollectors replaces the collector display list from config.Collector entries.
func (p *CollectorPanel) SetCollectors(collectors []config.Collector) {
	p.collectors = make([]CollectorDisplay, len(collectors))
	for i, c := range collectors {
		p.collectors[i] = CollectorDisplay{
			Name:      c.Name,
			Address:   c.Address,
			Version:   c.Version,
			Connected: true, // assume connected initially
		}
	}
	if p.cursor >= len(p.collectors) {
		p.cursor = max(0, len(p.collectors)-1)
	}
}

// SetSize updates the available width and height for the panel.
func (p *CollectorPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// UpdateStats applies exporter statistics to the matching collector display.
func (p *CollectorPanel) UpdateStats(stats map[string]*engine.ExporterStats) {
	for i := range p.collectors {
		if s, ok := stats[p.collectors[i].Name]; ok {
			p.collectors[i].PacketsSent = s.PacketsSent
			p.collectors[i].BytesSent = s.BytesSent
			p.collectors[i].Errors = s.Errors
			p.collectors[i].Connected = s.Errors == 0
		}
	}
}

// Update handles key messages when the panel is focused.
func (p *CollectorPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch p.mode {
		case collectorNormal:
			return p.updateNormal(msg)
		case collectorAdding, collectorEditing:
			return p.updateForm(msg)
		case collectorDeleting:
			return p.updateDelete(msg)
		}
	}
	return nil
}

// -- Normal mode ----------------------------------------------------------

func (p *CollectorPanel) updateNormal(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		if p.cursor < len(p.collectors)-1 {
			p.cursor++
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "n":
		p.enterAddMode()
	case "e":
		if len(p.collectors) > 0 {
			p.enterEditMode()
		}
	case "d":
		if len(p.collectors) > 0 {
			p.mode = collectorDeleting
		}
	}
	return nil
}

func (p *CollectorPanel) enterEditMode() {
	c := p.collectors[p.cursor]
	p.mode = collectorEditing
	p.formFocus = 0
	p.editName = c.Name
	p.nameInput.SetValue(c.Name)
	p.addressInput.SetValue(c.Address)
	p.versionInput.SetValue(c.Version)
	p.nameInput.Focus()
	p.addressInput.Blur()
	p.versionInput.Blur()
}

func (p *CollectorPanel) enterAddMode() {
	p.mode = collectorAdding
	p.formFocus = 0
	p.nameInput.SetValue("")
	p.addressInput.SetValue("")
	p.versionInput.SetValue("v9")
	p.nameInput.Focus()
	p.addressInput.Blur()
	p.versionInput.Blur()
}

// -- Form mode (add) ------------------------------------------------------

const collectorFormFieldCount = 3

func (p *CollectorPanel) updateForm(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.mode = collectorNormal
		return nil
	case "tab", "down":
		p.formFocus = (p.formFocus + 1) % collectorFormFieldCount
		p.focusField()
		return nil
	case "shift+tab", "up":
		p.formFocus = (p.formFocus + collectorFormFieldCount - 1) % collectorFormFieldCount
		p.focusField()
		return nil
	case "enter":
		c, ok := p.validateForm()
		if !ok {
			return nil
		}
		cd := CollectorDisplay{
			Name:      c.Name,
			Address:   c.Address,
			Version:   c.Version,
			Connected: true,
		}
		if p.mode == collectorAdding {
			p.collectors = append(p.collectors, cd)
			p.cursor = len(p.collectors) - 1
		} else {
			for i := range p.collectors {
				if p.collectors[i].Name == p.editName {
					p.collectors[i] = cd
					break
				}
			}
		}
		p.mode = collectorNormal
		return collectorChangedCmd(c)
	}

	// Forward to the focused textinput.
	p.updateFocusedInput(msg)
	return nil
}

func (p *CollectorPanel) focusField() {
	p.nameInput.Blur()
	p.addressInput.Blur()
	p.versionInput.Blur()
	switch p.formFocus {
	case 0:
		p.nameInput.Focus()
	case 1:
		p.addressInput.Focus()
	case 2:
		p.versionInput.Focus()
	}
}

func (p *CollectorPanel) updateFocusedInput(msg tea.KeyMsg) {
	switch p.formFocus {
	case 0:
		m, _ := p.nameInput.Update(msg)
		p.nameInput = m
	case 1:
		m, _ := p.addressInput.Update(msg)
		p.addressInput = m
	case 2:
		m, _ := p.versionInput.Update(msg)
		p.versionInput = m
	}
}

func (p *CollectorPanel) validateForm() (config.Collector, bool) {
	name := strings.TrimSpace(p.nameInput.Value())
	address := strings.TrimSpace(p.addressInput.Value())
	version := strings.TrimSpace(p.versionInput.Value())

	if name == "" || address == "" || version == "" {
		return config.Collector{}, false
	}

	// Normalize version.
	version = strings.ToLower(version)
	if version != "v9" && version != "ipfix" {
		return config.Collector{}, false
	}

	return config.Collector{
		Name:    name,
		Address: address,
		Version: version,
	}, true
}

// -- Delete confirm mode --------------------------------------------------

func (p *CollectorPanel) updateDelete(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "y":
		if p.cursor < len(p.collectors) {
			removed := p.collectors[p.cursor]
			p.collectors = append(p.collectors[:p.cursor], p.collectors[p.cursor+1:]...)
			if p.cursor >= len(p.collectors) && p.cursor > 0 {
				p.cursor--
			}
			p.mode = collectorNormal
			return collectorDeletedCmd(removed)
		}
		p.mode = collectorNormal
	case "n", "esc":
		p.mode = collectorNormal
	}
	return nil
}

// -- Commands -------------------------------------------------------------

// CollectorChangedMsg is emitted when a collector is added.
type CollectorChangedMsg struct {
	Collector config.Collector
}

// CollectorDeletedMsg is emitted when a collector is deleted.
type CollectorDeletedMsg struct {
	Collector CollectorDisplay
}

func collectorChangedCmd(c config.Collector) tea.Cmd {
	return func() tea.Msg {
		return CollectorChangedMsg{Collector: c}
	}
}

func collectorDeletedCmd(c CollectorDisplay) tea.Cmd {
	return func() tea.Msg {
		return CollectorDeletedMsg{Collector: c}
	}
}

// -- View -----------------------------------------------------------------

// View renders the collector panel as a status bar.
func (p *CollectorPanel) View() string {
	if p.mode == collectorAdding || p.mode == collectorEditing {
		return p.renderForm()
	}
	if p.mode == collectorDeleting {
		return p.renderDeleteConfirm()
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render("COLLECTORS"))
	b.WriteString("\n")

	if len(p.collectors) == 0 {
		b.WriteString("  No collectors configured\n")
	} else {
		b.WriteString(p.renderList())
	}

	// Footer
	b.WriteString(p.renderFooter())

	return b.String()
}

func (p *CollectorPanel) renderList() string {
	var b strings.Builder
	for i, c := range p.collectors {
		line := p.renderCollectorRow(c)
		if i == p.cursor {
			b.WriteString(activeItemStyle.Render("▸ " + line))
		} else if i%2 == 0 {
			// Scan line effect: even rows are dimmed.
			b.WriteString(dimItemStyle.Render("  " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (p *CollectorPanel) renderCollectorRow(c CollectorDisplay) string {
	// Status indicator
	status := "\u25cf OK"
	if c.Errors > 0 || !c.Connected {
		status = "\u25cf ERR"
	}

	// Format packet count
	pktStr := formatCount(c.PacketsSent)

	return fmt.Sprintf("%-20s \u2192 %-24s  %s  \u2191 %s pkt",
		truncateCollector(c.Address, 20),
		truncateCollector(c.Name, 24),
		status,
		pktStr)
}

func (p *CollectorPanel) renderForm() string {
	var b strings.Builder
	label := "NEW COLLECTOR"
	if p.mode == collectorEditing {
		label = "EDIT COLLECTOR"
	}
	b.WriteString(activeItemStyle.Render(label))
	b.WriteString("\n\n")

	fields := []struct {
		label string
		view  string
	}{
		{"Name:    ", p.nameInput.View()},
		{"Address: ", p.addressInput.View()},
		{"Version: ", p.versionInput.View()},
	}
	for i, f := range fields {
		prefix := "  "
		if i == p.formFocus {
			prefix = "\u25b8 "
		}
		b.WriteString(prefix + f.label + f.view + "\n")
	}
	b.WriteString("\n  Enter: save  Esc: cancel  Tab: next field\n")
	return b.String()
}

func (p *CollectorPanel) renderDeleteConfirm() string {
	if p.cursor >= len(p.collectors) {
		return ""
	}
	c := p.collectors[p.cursor]
	var b strings.Builder
	b.WriteString(activeItemStyle.Render("DELETE COLLECTOR"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Remove %q?\n\n", c.Name))
	b.WriteString("  Are you sure? [y/n]\n")
	return b.String()
}

func (p *CollectorPanel) renderFooter() string {
	return lipgloss.NewStyle().
		Foreground(colorAccent).
		Render("[N]ew  [E]dit  [D]elete")
}

// -- Helpers --------------------------------------------------------------

// formatCount formats a number with K/M/G suffixes.
func formatCount(n uint64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fG", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// truncateCollector shortens a string to maxLen.
func truncateCollector(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
