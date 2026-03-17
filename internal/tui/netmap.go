// internal/tui/netmap.go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/masterphelps/flowboy/internal/config"
)

// MapPanel renders an ASCII network topology map showing segments,
// machines, and active flows — matching the web GUI's Network Map panel.
type MapPanel struct {
	segments []config.Segment
	flows    []FlowDisplay
	width    int
	height   int
	tick     int
}

// NewMapPanel returns an initialised MapPanel.
func NewMapPanel() MapPanel {
	return MapPanel{}
}

// SetSize updates the available width and height.
func (p *MapPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// SetData updates the map with current segments and flows.
func (p *MapPanel) SetData(segments []config.Segment, flows []FlowDisplay) {
	p.segments = segments
	p.flows = flows
}

// Tick increments the animation frame counter.
func (p *MapPanel) Tick() {
	p.tick++
}

// View renders the network map.
func (p *MapPanel) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("NETWORK MAP"))
	b.WriteString("\n")

	if len(p.segments) == 0 {
		b.WriteString("\n  No machines configured\n")
		b.WriteString("  Add machines to see the network map\n")
		return b.String()
	}

	// Render each segment as a box
	boxWidth := p.width - 4
	if boxWidth < 40 {
		boxWidth = 40
	}

	segStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Foreground(colorGreen).
		Width(boxWidth).
		Padding(0, 1)

	for _, seg := range p.segments {
		cidr := seg.CIDR.String()
		if cidr == "0.0.0.0/0" {
			cidr = "PUBLIC"
		}

		var segContent strings.Builder
		segContent.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(cidr))
		segContent.WriteString("\n")

		for i, m := range seg.Machines {
			ones, _ := m.Mask.Size()
			icon := "■"
			line := fmt.Sprintf("  %s %-18s %s/%d", icon, m.Name, m.IP.String(), ones)
			if i%2 == 0 {
				segContent.WriteString(dimItemStyle.Render(line))
			} else {
				segContent.WriteString(line)
			}
			if i < len(seg.Machines)-1 {
				segContent.WriteString("\n")
			}
		}

		b.WriteString(segStyle.Render(segContent.String()))
		b.WriteString("\n")
	}

	// Draw backbone if multiple segments
	if len(p.segments) > 1 {
		backbone := strings.Repeat("═", boxWidth-4)
		backboneStr := fmt.Sprintf("  ╠%s╣", backbone)
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render("  PHYSICAL NETWORK"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorBorder).Render(backboneStr))
		b.WriteString("\n\n")
	}

	// Render active flows
	activeFlows := p.getActiveFlows()
	if len(activeFlows) > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("  ACTIVE FLOWS"))
		b.WriteString("\n")
		divider := "  " + strings.Repeat("─", boxWidth-2)
		b.WriteString(lipgloss.NewStyle().Foreground(colorBorder).Render(divider))
		b.WriteString("\n")

		for i, f := range activeFlows {
			wave := p.flowWave(f)
			status := lipgloss.NewStyle().Foreground(colorBright).Render("●")
			if !f.Enabled {
				status = lipgloss.NewStyle().Foreground(colorBorder).Render("○")
				wave = "───"
			}

			line := fmt.Sprintf("  %s %-8s:%-5d ─%s→ %-8s:%-5d  %s  %s",
				status,
				truncate(f.Source, 8), f.SrcPort,
				wave,
				truncate(f.Dest, 8), f.DstPort,
				f.Protocol,
				f.Rate)

			if i%2 == 0 {
				b.WriteString(dimItemStyle.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("\n  No active flows\n")
	}

	return b.String()
}

func (p *MapPanel) getActiveFlows() []FlowDisplay {
	var result []FlowDisplay
	for _, f := range p.flows {
		result = append(result, f)
	}
	return result
}

func (p *MapPanel) flowWave(f FlowDisplay) string {
	if !f.Active || !f.Enabled {
		return "───"
	}
	chars := []rune{'~', '∿'}
	frames := [][]int{
		{0, 1, 1, 0},
		{1, 1, 0, 1},
		{1, 0, 1, 1},
		{0, 1, 0, 1},
	}
	frame := p.tick % len(frames)
	pattern := frames[frame]
	var sb strings.Builder
	for i := 0; i < 3; i++ {
		sb.WriteRune(chars[pattern[i%len(pattern)]])
	}
	return sb.String()
}
