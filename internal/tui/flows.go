// internal/tui/flows.go
package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/masterphelps/flowboy/internal/config"
	"github.com/masterphelps/flowboy/internal/engine"
)

// flowPanelMode describes what the flow panel is currently doing.
type flowPanelMode int

const (
	flowNormal    flowPanelMode = iota
	flowAdding                  // inline form for new flow
	flowEditing                 // inline form for editing selected flow
	flowDeleting                // "Are you sure? y/n" prompt
)

// tickMsg is sent periodically to drive waveform animation.
type tickMsg time.Time

// flowStatsMsg wraps a FlowStats update from the engine.
type flowStatsMsg engine.FlowStats

// FlowDisplay holds display-ready information for a single flow row.
type FlowDisplay struct {
	Name        string
	Source      string
	SrcPort     uint16
	Dest        string
	DstPort     uint16
	Protocol    string
	Rate        string
	BytesSent   uint64
	PacketsSent uint64
	Active      bool
	Enabled     bool
}

// FlowPanel is a sub-model that renders the active flows list and handles
// CRUD operations via inline forms.
type FlowPanel struct {
	flows        []FlowDisplay
	cursor       int
	mode         flowPanelMode
	width        int
	height       int
	tick         int // animation frame counter
	nameInput    textinput.Model
	srcInput     textinput.Model // source machine name
	srcPortInput textinput.Model
	dstInput     textinput.Model // dest machine name
	dstPortInput textinput.Model
	protoInput   textinput.Model
	rateInput    textinput.Model
	appIDInput   textinput.Model
	formFocus    int
	editName     string // original name when editing
}

// NewFlowPanel returns an initialised FlowPanel.
func NewFlowPanel() FlowPanel {
	ni := textinput.New()
	ni.Placeholder = "flow-name"
	ni.CharLimit = 40
	ni.Width = 20

	si := textinput.New()
	si.Placeholder = "source-machine"
	si.CharLimit = 40
	si.Width = 20

	spi := textinput.New()
	spi.Placeholder = "46578"
	spi.CharLimit = 5
	spi.Width = 6

	di := textinput.New()
	di.Placeholder = "dest-machine"
	di.CharLimit = 40
	di.Width = 20

	dpi := textinput.New()
	dpi.Placeholder = "5432"
	dpi.CharLimit = 5
	dpi.Width = 6

	pi := textinput.New()
	pi.Placeholder = "TCP"
	pi.CharLimit = 4
	pi.Width = 6

	ri := textinput.New()
	ri.Placeholder = "90Mbps"
	ri.CharLimit = 20
	ri.Width = 12

	ai := textinput.New()
	ai.Placeholder = "0"
	ai.CharLimit = 10
	ai.Width = 10

	return FlowPanel{
		nameInput:    ni,
		srcInput:     si,
		srcPortInput: spi,
		dstInput:     di,
		dstPortInput: dpi,
		protoInput:   pi,
		rateInput:    ri,
		appIDInput:   ai,
	}
}

// SetFlows replaces the flow display list from engine config.Flow entries.
func (p *FlowPanel) SetFlows(flows []config.Flow) {
	p.flows = make([]FlowDisplay, len(flows))
	for i, f := range flows {
		p.flows[i] = FlowDisplay{
			Name:     f.Name,
			Source:   f.SourceName,
			SrcPort:  f.SourcePort,
			Dest:     f.DestName,
			DstPort:  f.DestPort,
			Protocol: f.Protocol,
			Rate:     f.Rate,
			Active:   f.Enabled,
			Enabled:  f.Enabled,
		}
	}
	if p.cursor >= len(p.flows) {
		p.cursor = max(0, len(p.flows)-1)
	}
}

// SetSize updates the available width and height.
func (p *FlowPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// Tick increments the animation frame counter.
func (p *FlowPanel) Tick() {
	p.tick++
}

// UpdateStats applies a FlowStats update to the matching flow display.
func (p *FlowPanel) UpdateStats(s engine.FlowStats) {
	for i := range p.flows {
		if p.flows[i].Name == s.FlowName {
			p.flows[i].BytesSent = s.BytesSent
			p.flows[i].PacketsSent = s.PacketsSent
			p.flows[i].Active = s.Active
			return
		}
	}
}

// Update handles key messages when the panel is focused.
func (p *FlowPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch p.mode {
		case flowNormal:
			return p.updateNormal(msg)
		case flowAdding, flowEditing:
			return p.updateForm(msg)
		case flowDeleting:
			return p.updateDelete(msg)
		}
	}
	return nil
}

// -- Normal mode ----------------------------------------------------------

func (p *FlowPanel) updateNormal(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		if p.cursor < len(p.flows)-1 {
			p.cursor++
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "n":
		p.enterAddMode()
	case "e":
		if len(p.flows) > 0 {
			p.enterEditMode()
		}
	case "d":
		if len(p.flows) > 0 {
			p.mode = flowDeleting
		}
	case "s":
		return p.startAllCmd()
	case "x":
		return p.stopAllCmd()
	case " ":
		if len(p.flows) > 0 {
			p.flows[p.cursor].Enabled = !p.flows[p.cursor].Enabled
			return p.flowToggleCmd()
		}
	}
	return nil
}

func (p *FlowPanel) enterAddMode() {
	p.mode = flowAdding
	p.formFocus = 0
	p.nameInput.SetValue("")
	p.srcInput.SetValue("")
	p.srcPortInput.SetValue("")
	p.dstInput.SetValue("")
	p.dstPortInput.SetValue("")
	p.protoInput.SetValue("TCP")
	p.rateInput.SetValue("")
	p.appIDInput.SetValue("0")
	p.nameInput.Focus()
	p.blurAllExcept(0)
}

func (p *FlowPanel) enterEditMode() {
	f := p.flows[p.cursor]
	p.mode = flowEditing
	p.formFocus = 0
	p.editName = f.Name
	p.nameInput.SetValue(f.Name)
	p.srcInput.SetValue(f.Source)
	p.srcPortInput.SetValue(strconv.Itoa(int(f.SrcPort)))
	p.dstInput.SetValue(f.Dest)
	p.dstPortInput.SetValue(strconv.Itoa(int(f.DstPort)))
	p.protoInput.SetValue(f.Protocol)
	p.rateInput.SetValue(f.Rate)
	p.appIDInput.SetValue("0")
	p.nameInput.Focus()
	p.blurAllExcept(0)
}

const flowFormFieldCount = 8

// -- Form mode (add / edit) -----------------------------------------------

func (p *FlowPanel) updateForm(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.mode = flowNormal
		return nil
	case "tab", "down":
		p.formFocus = (p.formFocus + 1) % flowFormFieldCount
		p.focusField()
		return nil
	case "shift+tab", "up":
		p.formFocus = (p.formFocus + flowFormFieldCount - 1) % flowFormFieldCount
		p.focusField()
		return nil
	case "enter":
		f, ok := p.validateForm()
		if !ok {
			return nil
		}
		fd := FlowDisplay{
			Name:     f.Name,
			Source:   f.SourceName,
			SrcPort:  f.SourcePort,
			Dest:     f.DestName,
			DstPort:  f.DestPort,
			Protocol: f.Protocol,
			Rate:     f.Rate,
			Active:   f.Enabled,
			Enabled:  f.Enabled,
		}
		if p.mode == flowAdding {
			p.flows = append(p.flows, fd)
			p.cursor = len(p.flows) - 1
		} else {
			for i := range p.flows {
				if p.flows[i].Name == p.editName {
					p.flows[i] = fd
					break
				}
			}
		}
		oldName := ""
		if p.mode == flowEditing {
			oldName = p.editName
		}
		p.mode = flowNormal
		return flowChangedCmd(f, oldName)
	}

	// Forward to the focused textinput.
	p.updateFocusedInput(msg)
	return nil
}

func (p *FlowPanel) focusField() {
	p.blurAllExcept(p.formFocus)
}

func (p *FlowPanel) blurAllExcept(focus int) {
	p.nameInput.Blur()
	p.srcInput.Blur()
	p.srcPortInput.Blur()
	p.dstInput.Blur()
	p.dstPortInput.Blur()
	p.protoInput.Blur()
	p.rateInput.Blur()
	p.appIDInput.Blur()
	switch focus {
	case 0:
		p.nameInput.Focus()
	case 1:
		p.srcInput.Focus()
	case 2:
		p.srcPortInput.Focus()
	case 3:
		p.dstInput.Focus()
	case 4:
		p.dstPortInput.Focus()
	case 5:
		p.protoInput.Focus()
	case 6:
		p.rateInput.Focus()
	case 7:
		p.appIDInput.Focus()
	}
}

func (p *FlowPanel) updateFocusedInput(msg tea.KeyMsg) {
	switch p.formFocus {
	case 0:
		m, _ := p.nameInput.Update(msg)
		p.nameInput = m
	case 1:
		m, _ := p.srcInput.Update(msg)
		p.srcInput = m
	case 2:
		m, _ := p.srcPortInput.Update(msg)
		p.srcPortInput = m
	case 3:
		m, _ := p.dstInput.Update(msg)
		p.dstInput = m
	case 4:
		m, _ := p.dstPortInput.Update(msg)
		p.dstPortInput = m
	case 5:
		m, _ := p.protoInput.Update(msg)
		p.protoInput = m
	case 6:
		m, _ := p.rateInput.Update(msg)
		p.rateInput = m
	case 7:
		m, _ := p.appIDInput.Update(msg)
		p.appIDInput = m
	}
}

func (p *FlowPanel) validateForm() (config.Flow, bool) {
	name := strings.TrimSpace(p.nameInput.Value())
	src := strings.TrimSpace(p.srcInput.Value())
	srcPort := strings.TrimSpace(p.srcPortInput.Value())
	dst := strings.TrimSpace(p.dstInput.Value())
	dstPort := strings.TrimSpace(p.dstPortInput.Value())
	proto := strings.TrimSpace(p.protoInput.Value())
	rate := strings.TrimSpace(p.rateInput.Value())
	appIDStr := strings.TrimSpace(p.appIDInput.Value())

	if name == "" || src == "" || srcPort == "" || dst == "" || dstPort == "" || proto == "" || rate == "" {
		return config.Flow{}, false
	}

	sp, err := strconv.ParseUint(srcPort, 10, 16)
	if err != nil {
		return config.Flow{}, false
	}
	dp, err := strconv.ParseUint(dstPort, 10, 16)
	if err != nil {
		return config.Flow{}, false
	}

	// Validate that the rate is parseable.
	_, err = config.ParseRate(rate)
	if err != nil {
		return config.Flow{}, false
	}

	var appID uint32
	if appIDStr != "" {
		a, err := strconv.ParseUint(appIDStr, 10, 32)
		if err != nil {
			return config.Flow{}, false
		}
		appID = uint32(a)
	}

	f := config.NewFlow()
	f.Name = name
	f.SourceName = src
	f.SourcePort = uint16(sp)
	f.DestName = dst
	f.DestPort = uint16(dp)
	f.Protocol = strings.ToUpper(proto)
	f.Rate = rate
	f.AppID = appID
	return f, true
}

// -- Delete confirm mode --------------------------------------------------

func (p *FlowPanel) updateDelete(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "y":
		if p.cursor < len(p.flows) {
			removed := p.flows[p.cursor]
			p.flows = append(p.flows[:p.cursor], p.flows[p.cursor+1:]...)
			if p.cursor >= len(p.flows) && p.cursor > 0 {
				p.cursor--
			}
			p.mode = flowNormal
			return flowDeletedCmd(removed)
		}
		p.mode = flowNormal
	case "n", "esc":
		p.mode = flowNormal
	}
	return nil
}

// -- Commands -------------------------------------------------------------

// FlowChangedMsg is emitted when a flow is added or edited.
type FlowChangedMsg struct {
	Flow    config.Flow
	OldName string // non-empty when editing
}

// FlowDeletedMsg is emitted when a flow is deleted.
type FlowDeletedMsg struct {
	Flow FlowDisplay
}

// FlowToggleMsg is emitted when a single flow is toggled on/off.
type FlowToggleMsg struct {
	Flow FlowDisplay
}

// FlowStartAllMsg requests starting all flows.
type FlowStartAllMsg struct{}

// FlowStopAllMsg requests stopping all flows.
type FlowStopAllMsg struct{}

func flowChangedCmd(f config.Flow, oldName string) tea.Cmd {
	return func() tea.Msg {
		return FlowChangedMsg{Flow: f, OldName: oldName}
	}
}

func flowDeletedCmd(f FlowDisplay) tea.Cmd {
	return func() tea.Msg {
		return FlowDeletedMsg{Flow: f}
	}
}

func (p *FlowPanel) flowToggleCmd() tea.Cmd {
	f := p.flows[p.cursor]
	return func() tea.Msg {
		return FlowToggleMsg{Flow: f}
	}
}

func (p *FlowPanel) startAllCmd() tea.Cmd {
	for i := range p.flows {
		p.flows[i].Enabled = true
	}
	return func() tea.Msg {
		return FlowStartAllMsg{}
	}
}

func (p *FlowPanel) stopAllCmd() tea.Cmd {
	for i := range p.flows {
		p.flows[i].Enabled = false
	}
	return func() tea.Msg {
		return FlowStopAllMsg{}
	}
}

// tickCmd returns a tea.Cmd that fires a tickMsg after the given duration.
func tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// waitForStatsCmd returns a tea.Cmd that reads from the stats channel and
// wraps the result in a flowStatsMsg.
func waitForStatsCmd(ch <-chan engine.FlowStats) tea.Cmd {
	return func() tea.Msg {
		s, ok := <-ch
		if !ok {
			return nil
		}
		return flowStatsMsg(s)
	}
}

// -- View -----------------------------------------------------------------

// View renders the flow list panel contents (without the outer border).
func (p *FlowPanel) View() string {
	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render("ACTIVE FLOWS"))
	b.WriteString("\n")

	if p.mode == flowAdding || p.mode == flowEditing {
		b.WriteString(p.renderForm())
	} else if p.mode == flowDeleting {
		b.WriteString(p.renderDeleteConfirm())
	} else if len(p.flows) == 0 {
		b.WriteString("  No active flows\n")
	} else {
		b.WriteString(p.renderList())
	}

	// Pad to fill available height so footer stays at the bottom.
	lines := strings.Count(b.String(), "\n")
	remaining := p.height - lines - 2
	for i := 0; i < remaining; i++ {
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(p.renderFooter())

	return b.String()
}

func (p *FlowPanel) renderList() string {
	var b strings.Builder
	for i, f := range p.flows {
		line := p.renderFlowRow(f, i)
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

func (p *FlowPanel) renderFlowRow(f FlowDisplay, _ int) string {
	// source:port -> dest:port  PROTO  [bar] rate  [waveform]
	srcDst := fmt.Sprintf("%-8s:%d", truncate(f.Source, 8), f.SrcPort)
	arrow := " \u2192 "
	dstPart := fmt.Sprintf("%-8s:%d", truncate(f.Dest, 8), f.DstPort)

	bar := p.progressBar(f)
	wave := ""
	if f.Active && f.Enabled {
		wave = p.waveform(f)
	}

	status := " "
	if !f.Enabled {
		status = "\u2718" // cross mark
	}

	return fmt.Sprintf("%s%s%s  %s  %s %s  %s %s",
		srcDst, arrow, dstPart,
		f.Protocol, bar, f.Rate, wave, status)
}

func (p *FlowPanel) progressBar(f FlowDisplay) string {
	const barWidth = 10
	// Parse configured rate to get max bps.
	rate, err := config.ParseRate(f.Rate)
	var ratio float64
	if err == nil && rate.BitsPerSecond > 0 && f.Active && f.Enabled {
		// Estimate current bps from bytes sent (simplistic: assume last
		// interval was 1 second).
		if f.BytesSent > 0 {
			// Show a reasonable fill based on activity.
			ratio = 0.6 + 0.3*float64(f.BytesSent%100)/100.0
		} else {
			ratio = 0.5
		}
	}
	if !f.Enabled {
		ratio = 0
	}
	filled := int(ratio * barWidth)
	if filled > barWidth {
		filled = barWidth
	}
	return strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", barWidth-filled)
}

// waveform generates an oscilloscope-style animation that cycles on each tick.
func (p *FlowPanel) waveform(f FlowDisplay) string {
	if !f.Active || !f.Enabled {
		return ""
	}

	// Waveform length scales with rate string length as a proxy for throughput.
	waveLen := 4
	rate, err := config.ParseRate(f.Rate)
	if err == nil {
		switch {
		case rate.BitsPerSecond >= 1_000_000_000:
			waveLen = 8
		case rate.BitsPerSecond >= 100_000_000:
			waveLen = 6
		case rate.BitsPerSecond >= 10_000_000:
			waveLen = 5
		default:
			waveLen = 4
		}
	}

	chars := []rune{'~', '\u223F'} // ~ and ∿
	frames := [][]int{
		{0, 1, 1, 0}, // ~∿∿~
		{1, 1, 0, 1}, // ∿∿~∿
		{1, 0, 1, 1}, // ∿~∿∿
		{0, 1, 0, 1}, // ~∿~∿
	}

	frame := p.tick % len(frames)
	pattern := frames[frame]

	var sb strings.Builder
	for i := 0; i < waveLen; i++ {
		idx := pattern[i%len(pattern)]
		sb.WriteRune(chars[idx])
	}
	return sb.String()
}

func (p *FlowPanel) renderForm() string {
	var b strings.Builder
	label := "NEW FLOW"
	if p.mode == flowEditing {
		label = "EDIT FLOW"
	}
	b.WriteString(activeItemStyle.Render(label))
	b.WriteString("\n\n")

	fields := []struct {
		label string
		view  string
	}{
		{"Name:     ", p.nameInput.View()},
		{"Source:   ", p.srcInput.View()},
		{"Src Port: ", p.srcPortInput.View()},
		{"Dest:     ", p.dstInput.View()},
		{"Dst Port: ", p.dstPortInput.View()},
		{"Protocol: ", p.protoInput.View()},
		{"Rate:     ", p.rateInput.View()},
		{"App ID:   ", p.appIDInput.View()},
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

func (p *FlowPanel) renderDeleteConfirm() string {
	if p.cursor >= len(p.flows) {
		return ""
	}
	f := p.flows[p.cursor]
	var b strings.Builder
	b.WriteString(activeItemStyle.Render("DELETE FLOW"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Remove %q?\n\n", f.Name))
	b.WriteString("  Are you sure? [y/n]\n")
	return b.String()
}

func (p *FlowPanel) renderFooter() string {
	return lipgloss.NewStyle().
		Foreground(colorAccent).
		Render("[N]ew  [E]dit  [D]elete  [S]tart All  [X] Stop All")
}

// truncate shortens a string to maxLen, appending nothing (just clips).
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
