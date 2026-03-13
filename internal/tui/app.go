// internal/tui/app.go
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/masterphelps/flowboy/internal/config"
	"github.com/masterphelps/flowboy/internal/engine"
)

type focusPanel int

const (
	focusMachines focusPanel = iota
	focusFlows
	focusCollectors
)

// Model is the top-level Bubbletea model that composes the three-panel
// Pip-Boy TUI layout: machines (left), active flows (right), and
// collectors (bottom status bar).
type Model struct {
	engine         *engine.Engine
	cfg            *config.Config
	configPath     string
	machinePanel   MachinePanel
	flowPanel      FlowPanel
	collectorPanel CollectorPanel
	width          int
	height         int
	focus          focusPanel
	quitting       bool
}

// NewModel creates a new TUI model wired to the given engine.
// It takes the engine, the loaded config (for persistence), and the
// config file path (for saving changes back to YAML).
func NewModel(eng *engine.Engine, cfg *config.Config, configPath string) Model {
	mp := NewMachinePanel()
	fp := NewFlowPanel()
	cp := NewCollectorPanel()
	// Seed the panels with whatever the engine already knows about.
	if eng != nil {
		mp.SetMachines(eng.Machines())
		fp.SetFlows(eng.Flows())
	}
	// Load collectors from config (collectors are config-only, not engine objects).
	if cfg != nil {
		cp.SetCollectors(cfg.Collectors)
	}
	return Model{
		engine:         eng,
		cfg:            cfg,
		configPath:     configPath,
		machinePanel:   mp,
		flowPanel:      fp,
		collectorPanel: cp,
		focus:          focusMachines,
	}
}

// Init satisfies the tea.Model interface. Start the tick and stats listener.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd()}
	if m.engine != nil {
		cmds = append(cmds, waitForStatsCmd(m.engine.Stats()))
	}
	return tea.Batch(cmds...)
}

// Update handles keyboard input and window resize events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global keys that always work regardless of focus or sub-mode.
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

		// When the machine panel is in a modal mode (form / delete confirm),
		// all keys go to it — don't let tab/q escape.
		if m.focus == focusMachines && m.machinePanel.mode != machineNormal {
			cmd := m.machinePanel.Update(msg)
			return m, cmd
		}

		// When the flow panel is in a modal mode, all keys go to it.
		if m.focus == focusFlows && m.flowPanel.mode != flowNormal {
			cmd := m.flowPanel.Update(msg)
			return m, cmd
		}

		// When the collector panel is in a modal mode, all keys go to it.
		if m.focus == focusCollectors && m.collectorPanel.mode != collectorNormal {
			cmd := m.collectorPanel.Update(msg)
			return m, cmd
		}

		// Global keys that work only in normal mode.
		switch msg.String() {
		case "q":
			m.quitting = true
			return m, tea.Quit
		case "tab":
			m.focus = (m.focus + 1) % 3
		case "shift+tab":
			m.focus = (m.focus + 2) % 3
		default:
			// Forward to the focused panel.
			switch m.focus {
			case focusMachines:
				cmd := m.machinePanel.Update(msg)
				return m, cmd
			case focusFlows:
				cmd := m.flowPanel.Update(msg)
				return m, cmd
			case focusCollectors:
				cmd := m.collectorPanel.Update(msg)
				return m, cmd
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updatePanelSizes()

	// Tick for waveform animation.
	case tickMsg:
		m.flowPanel.Tick()
		return m, tickCmd()

	// Stats from the engine.
	case flowStatsMsg:
		m.flowPanel.UpdateStats(engine.FlowStats(msg))
		var cmd tea.Cmd
		if m.engine != nil {
			cmd = waitForStatsCmd(m.engine.Stats())
		}
		return m, cmd

	// React to machine CRUD messages from the panel.
	case MachineChangedMsg:
		if m.engine != nil {
			if msg.OldName != "" {
				m.engine.UpdateMachine(msg.OldName, msg.Machine)
			} else {
				m.engine.AddMachine(msg.Machine)
			}
		}
		m.saveConfig()
	case MachineDeletedMsg:
		if m.engine != nil {
			m.engine.RemoveMachine(msg.Machine.Name)
		}
		m.saveConfig()

	// React to flow CRUD messages from the panel.
	case FlowChangedMsg:
		if m.engine != nil {
			if msg.OldName != "" {
				_ = m.engine.RemoveFlow(msg.OldName)
			}
			_ = m.engine.AddFlow(msg.Flow)
		}
		m.saveConfig()
	case FlowDeletedMsg:
		if m.engine != nil {
			_ = m.engine.RemoveFlow(msg.Flow.Name)
		}
		m.saveConfig()
	case FlowStartAllMsg:
		if m.engine != nil {
			m.engine.Start()
		}
	case FlowStopAllMsg:
		if m.engine != nil {
			m.engine.Stop()
		}

	// React to collector CRUD messages from the panel.
	case CollectorChangedMsg:
		m.saveConfig()
	case CollectorDeletedMsg:
		m.saveConfig()
	}
	return m, nil
}

func (m *Model) updatePanelSizes() {
	leftWidth := m.width / 3
	rightWidth := m.width - leftWidth - 4 // account for borders
	contentHeight := m.height - 8
	// Subtract panel border and padding (roughly 4 chars for border sides, 2 for padding).
	m.machinePanel.SetSize(leftWidth-2, contentHeight-2)
	m.flowPanel.SetSize(rightWidth-2, contentHeight-2)
	m.collectorPanel.SetSize(m.width-4, 6) // status bar height
}

// View renders the full TUI layout: title, left+right panels, status bar.
func (m Model) View() string {
	if m.quitting {
		return "Goodbye.\n"
	}
	if m.width == 0 {
		return "Loading..."
	}

	// Title
	title := titleStyle.Width(m.width).Render("F L O W B O Y  3 0 0 0")

	// Panel dimensions — account for borders (2 chars each side) and padding.
	leftWidth := m.width / 3
	rightWidth := m.width - leftWidth - 4 // account for borders
	contentHeight := m.height - 8         // title + status bar + borders

	// Machine panel (left) — rendered by the sub-model.
	machineStyle := panelStyle.Width(leftWidth).Height(contentHeight)
	if m.focus == focusMachines {
		machineStyle = machineStyle.BorderForeground(colorBright)
	} else {
		machineStyle = machineStyle.BorderForeground(colorBorder)
	}
	machinePanel := machineStyle.Render(m.machinePanel.View())

	// Flows panel (right) — rendered by the sub-model.
	flowStyle := panelStyle.Width(rightWidth).Height(contentHeight)
	if m.focus == focusFlows {
		flowStyle = flowStyle.BorderForeground(colorBright)
	} else {
		flowStyle = flowStyle.BorderForeground(colorBorder)
	}
	flowPanel := flowStyle.Render(m.flowPanel.View())

	// Join left + right
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, machinePanel, flowPanel)

	// Collector status bar
	collectorBar := m.renderCollectorBar()

	return lipgloss.JoinVertical(lipgloss.Left, title, mainContent, collectorBar)
}

// renderCollectorBar draws the bottom collector status bar using the
// CollectorPanel sub-model.
func (m Model) renderCollectorBar() string {
	barStyle := panelStyle.Width(m.width - 2).
		BorderForeground(colorBorder)
	if m.focus == focusCollectors {
		barStyle = barStyle.BorderForeground(colorBright)
	}
	content := m.collectorPanel.View()

	// Append engine status line.
	engineLine := fmt.Sprintf("  ENGINE: %s  |  Tab: switch panels  q: quit",
		engineStatus(m.engine))
	content += "\n" + lipgloss.NewStyle().Foreground(colorAccent).Render(engineLine)

	return barStyle.Render(content)
}

// engineStatus returns "ON" or "OFF" depending on engine state.
func engineStatus(eng *engine.Engine) string {
	if eng != nil && eng.Running() {
		return "ON"
	}
	return "OFF"
}

// saveConfig rebuilds the config from the current panel state and writes it to disk.
func (m *Model) saveConfig() {
	if m.cfg == nil || m.configPath == "" {
		return
	}

	// Rebuild machines from panel state.
	m.cfg.Machines = make([]config.MachineConfig, len(m.machinePanel.machines))
	for i, mach := range m.machinePanel.machines {
		ones, _ := mach.Mask.Size()
		m.cfg.Machines[i] = config.MachineConfig{
			Name: mach.Name,
			IP:   mach.IP.String(),
			Mask: ones,
		}
	}

	// Rebuild flows from panel state.
	m.cfg.Flows = make([]config.FlowConfig, len(m.flowPanel.flows))
	for i, fd := range m.flowPanel.flows {
		m.cfg.Flows[i] = config.FlowConfig{
			Name:        fd.Name,
			Source:      fd.Source,
			SourcePort:  fd.SrcPort,
			Destination: fd.Dest,
			DestPort:    fd.DstPort,
			Protocol:    fd.Protocol,
			Rate:        fd.Rate,
			Enabled:     fd.Enabled,
		}
	}

	// Rebuild collectors from panel state.
	m.cfg.Collectors = make([]config.Collector, len(m.collectorPanel.collectors))
	for i, cd := range m.collectorPanel.collectors {
		m.cfg.Collectors[i] = config.Collector{
			Name:    cd.Name,
			Address: cd.Address,
			Version: cd.Version,
		}
	}

	// Best-effort save; ignore errors in the TUI context.
	_ = config.SaveConfig(m.cfg, m.configPath)
}
