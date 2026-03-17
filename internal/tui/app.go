// internal/tui/app.go
package tui

import (
	"fmt"
	"path/filepath"
	"time"

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

type viewMode int

const (
	viewDashboard viewMode = iota // machines | flows + collectors bar
	viewMap                       // fullscreen network map
	viewConfig                    // config file management
)

// exporterStatsMsg wraps periodic exporter stats for collector panel updates.
type exporterStatsMsg map[string]*engine.ExporterStats

// Model is the top-level Bubbletea model that composes the multi-panel
// Pip-Boy TUI layout matching the web GUI's functionality.
type Model struct {
	engine         *engine.Engine
	exporter       *engine.Exporter
	cfg            *config.Config
	configPath     string
	machinePanel   MachinePanel
	flowPanel      FlowPanel
	collectorPanel CollectorPanel
	mapPanel       MapPanel
	configPanel    ConfigPanel
	width          int
	height         int
	focus          focusPanel
	view           viewMode
	quitting       bool
	// Aggregated stats for status bar
	totalBytes   uint64
	totalPackets uint64
	activeFlows  int
}

// NewModel creates a new TUI model wired to the given engine and exporter.
func NewModel(eng *engine.Engine, exp *engine.Exporter, cfg *config.Config, configPath string) Model {
	mp := NewMachinePanel()
	fp := NewFlowPanel()
	cp := NewCollectorPanel()
	np := NewMapPanel()
	cfp := NewConfigPanel(configPath)

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
		exporter:       exp,
		cfg:            cfg,
		configPath:     configPath,
		machinePanel:   mp,
		flowPanel:      fp,
		collectorPanel: cp,
		mapPanel:       np,
		configPanel:    cfp,
		focus:          focusMachines,
		view:           viewDashboard,
	}
}

// Init satisfies the tea.Model interface. Start the tick, stats listener, and exporter stats poller.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd()}
	if m.engine != nil {
		cmds = append(cmds, waitForStatsCmd(m.engine.Stats()))
	}
	if m.exporter != nil {
		cmds = append(cmds, pollExporterStatsCmd(m.exporter))
	}
	return tea.Batch(cmds...)
}

// pollExporterStatsCmd polls exporter stats every second.
func pollExporterStatsCmd(exp *engine.Exporter) tea.Cmd {
	return tea.Tick(1*time.Second, func(_ time.Time) tea.Msg {
		if exp == nil {
			return exporterStatsMsg(nil)
		}
		return exporterStatsMsg(exp.GetStats())
	})
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

		// When in map or config view, handle view-specific keys.
		if m.view == viewMap {
			switch msg.String() {
			case "esc", "q", "m":
				m.view = viewDashboard
				return m, nil
			}
			return m, nil
		}
		if m.view == viewConfig {
			switch msg.String() {
			case "esc", "f":
				m.view = viewDashboard
				return m, nil
			case "q":
				// Don't quit from config view, go back to dashboard
				m.view = viewDashboard
				return m, nil
			}
			// Forward to config panel
			cmd := m.configPanel.Update(msg)
			return m, cmd
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
			// Clear machine selection filter when leaving machines panel
			if m.focus != focusMachines {
				m.machinePanel.selected = false
				m.flowPanel.SetFilter("")
			}
		case "shift+tab":
			m.focus = (m.focus + 2) % 3
			if m.focus != focusMachines {
				m.machinePanel.selected = false
				m.flowPanel.SetFilter("")
			}
		case "m":
			// Toggle network map view
			m.refreshMapData()
			m.view = viewMap
			return m, nil
		case "f":
			// Toggle config/file management view
			m.configPanel.refreshList()
			m.view = viewConfig
			return m, nil
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
		m.mapPanel.Tick()
		return m, tickCmd()

	// Stats from the engine.
	case flowStatsMsg:
		stats := engine.FlowStats(msg)
		m.flowPanel.UpdateStats(stats)
		// Accumulate for status bar
		m.totalBytes = stats.BytesSent
		m.totalPackets = stats.PacketsSent
		var cmd tea.Cmd
		if m.engine != nil {
			cmd = waitForStatsCmd(m.engine.Stats())
		}
		return m, cmd

	// Exporter stats for collectors panel.
	case exporterStatsMsg:
		if msg != nil {
			m.collectorPanel.UpdateStats(map[string]*engine.ExporterStats(msg))
		}
		if m.exporter != nil {
			return m, pollExporterStatsCmd(m.exporter)
		}
		return m, nil

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

	// React to machine selection for cross-filtering.
	case MachineSelectedMsg:
		if msg.Selected && msg.Machine != nil {
			m.flowPanel.SetFilter(msg.Machine.Name)
		} else {
			m.flowPanel.SetFilter("")
		}

	// React to machine import.
	case MachineImportMsg:
		if m.engine != nil {
			for _, mach := range msg.Machines {
				m.engine.AddMachine(mach)
			}
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

	// Handle individual flow toggle (start/stop).
	case FlowToggleMsg:
		if m.engine != nil {
			// Remove and re-add with updated enabled state (matches web GUI behavior)
			_ = m.engine.RemoveFlow(msg.Flow.Name)
			f := config.NewFlow()
			f.Name = msg.Flow.Name
			f.SourceName = msg.Flow.Source
			f.SourcePort = msg.Flow.SrcPort
			f.DestName = msg.Flow.Dest
			f.DestPort = msg.Flow.DstPort
			f.Protocol = msg.Flow.Protocol
			f.Rate = msg.Flow.Rate
			f.AppID = msg.Flow.AppID
			f.Enabled = msg.Flow.Enabled
			_ = m.engine.AddFlow(f)
		}
		m.saveConfig()

	case FlowStartAllMsg:
		if m.engine != nil {
			m.engine.Start()
		}
		m.saveConfig()
	case FlowStopAllMsg:
		if m.engine != nil {
			m.engine.Stop()
		}
		m.saveConfig()

	// React to collector CRUD messages from the panel.
	case CollectorChangedMsg:
		m.saveConfig()
	case CollectorDeletedMsg:
		m.saveConfig()

	// Config management messages.
	case ConfigOpenMsg:
		m.handleConfigOpen(msg)
	case ConfigSaveMsg:
		m.saveConfig()
		m.configPanel.statusMsg = "Saved."
	case ConfigSaveAsMsg:
		m.configPath = msg.Path
		m.configPanel.SetConfigPath(msg.Path)
		m.saveConfig()
		m.configPanel.statusMsg = fmt.Sprintf("Saved as %s", msg.Name)
	case ConfigNewMsg:
		m.handleConfigNew()
	}
	return m, nil
}

func (m *Model) handleConfigOpen(msg ConfigOpenMsg) {
	// Stop engine
	if m.engine != nil {
		m.engine.Stop()
		// Remove existing flows and machines
		for _, f := range m.engine.Flows() {
			_ = m.engine.RemoveFlow(f.Name)
		}
		for _, mach := range m.engine.Machines() {
			m.engine.RemoveMachine(mach.Name)
		}
		// Load new machines
		for _, mc := range msg.Cfg.Machines {
			mach, err := mc.ToMachine()
			if err != nil {
				continue
			}
			m.engine.AddMachine(mach)
		}
		// Load new flows
		for _, fc := range msg.Cfg.Flows {
			f, err := fc.ToFlow()
			if err != nil {
				continue
			}
			_ = m.engine.AddFlow(f)
		}
	}

	// Update config and path
	*m.cfg = *msg.Cfg
	m.configPath = msg.Path
	m.configPanel.SetConfigPath(msg.Path)

	// Refresh panels
	if m.engine != nil {
		m.machinePanel.SetMachines(m.engine.Machines())
		m.flowPanel.SetFlows(m.engine.Flows())
	}
	m.collectorPanel.SetCollectors(msg.Cfg.Collectors)
	m.configPanel.statusMsg = fmt.Sprintf("Loaded %s", filepath.Base(msg.Path))
}

func (m *Model) handleConfigNew() {
	// Stop engine
	if m.engine != nil {
		m.engine.Stop()
		for _, f := range m.engine.Flows() {
			_ = m.engine.RemoveFlow(f.Name)
		}
		for _, mach := range m.engine.Machines() {
			m.engine.RemoveMachine(mach.Name)
		}
	}

	// Clear config
	m.cfg.Machines = nil
	m.cfg.Flows = nil
	m.cfg.Collectors = nil
	m.configPath = filepath.Join(filepath.Dir(m.configPath), "untitled.yaml")
	m.configPanel.SetConfigPath(m.configPath)

	// Refresh panels with empty data
	m.machinePanel.SetMachines(nil)
	m.flowPanel.SetFlows(nil)
	m.collectorPanel.SetCollectors(nil)
	m.configPanel.statusMsg = "New config."
}

func (m *Model) refreshMapData() {
	if m.cfg != nil {
		segments := m.cfg.BuildSegments()
		m.mapPanel.SetData(segments, m.flowPanel.flows)
	}
}

func (m *Model) updatePanelSizes() {
	switch m.view {
	case viewMap:
		m.mapPanel.SetSize(m.width-4, m.height-6)
	case viewConfig:
		m.configPanel.SetSize(m.width-4, m.height-6)
	default:
		leftWidth := m.width / 3
		rightWidth := m.width - leftWidth - 4
		contentHeight := m.height - 8
		m.machinePanel.SetSize(leftWidth-2, contentHeight-2)
		m.flowPanel.SetSize(rightWidth-2, contentHeight-2)
		m.collectorPanel.SetSize(m.width-4, 6)
	}
}

// View renders the full TUI layout.
func (m Model) View() string {
	if m.quitting {
		return "Goodbye.\n"
	}
	if m.width == 0 {
		return "Loading..."
	}

	// Title
	title := titleStyle.Width(m.width).Render("F L O W B O Y  3 0 0 0")

	switch m.view {
	case viewMap:
		return m.renderMapView(title)
	case viewConfig:
		return m.renderConfigView(title)
	default:
		return m.renderDashboard(title)
	}
}

func (m Model) renderDashboard(title string) string {
	leftWidth := m.width / 3
	rightWidth := m.width - leftWidth - 4
	contentHeight := m.height - 8

	// Machine panel (left)
	machineStyle := panelStyle.Width(leftWidth).Height(contentHeight)
	if m.focus == focusMachines {
		machineStyle = machineStyle.BorderForeground(colorBright)
	} else {
		machineStyle = machineStyle.BorderForeground(colorBorder)
	}
	machinePanel := machineStyle.Render(m.machinePanel.View())

	// Flows panel (right)
	flowStyle := panelStyle.Width(rightWidth).Height(contentHeight)
	if m.focus == focusFlows {
		flowStyle = flowStyle.BorderForeground(colorBright)
	} else {
		flowStyle = flowStyle.BorderForeground(colorBorder)
	}
	flowPanel := flowStyle.Render(m.flowPanel.View())

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, machinePanel, flowPanel)
	statusBar := m.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, title, mainContent, statusBar)
}

func (m Model) renderMapView(title string) string {
	mapStyle := panelStyle.Width(m.width - 2).Height(m.height - 6).
		BorderForeground(colorBright)
	content := m.mapPanel.View()
	return lipgloss.JoinVertical(lipgloss.Left, title, mapStyle.Render(content),
		m.renderNavHint())
}

func (m Model) renderConfigView(title string) string {
	cfgStyle := panelStyle.Width(m.width - 2).Height(m.height - 6).
		BorderForeground(colorBright)
	content := m.configPanel.View()
	return lipgloss.JoinVertical(lipgloss.Left, title, cfgStyle.Render(content),
		m.renderNavHint())
}

func (m Model) renderNavHint() string {
	return lipgloss.NewStyle().Foreground(colorAccent).
		Render("  [Esc] Dashboard  [M] Map  [F] File  [Q] Quit")
}

// renderStatusBar draws the bottom status bar with collectors, engine status,
// throughput, active flow count, and packets/sec.
func (m Model) renderStatusBar() string {
	barStyle := panelStyle.Width(m.width - 2).
		BorderForeground(colorBorder)
	if m.focus == focusCollectors {
		barStyle = barStyle.BorderForeground(colorBright)
	}
	content := m.collectorPanel.View()

	// Count active/enabled flows
	activeCount := 0
	var totalBps uint64
	for _, f := range m.flowPanel.flows {
		if f.Enabled {
			activeCount++
			rate, err := config.ParseRate(f.Rate)
			if err == nil {
				totalBps += rate.BitsPerSecond
			}
		}
	}

	// Format throughput
	throughput := formatThroughput(totalBps)

	// Estimate packets/sec from total throughput (assuming 800B avg packet)
	pktsPerSec := uint64(0)
	if totalBps > 0 {
		pktsPerSec = totalBps / 8 / 800
	}

	engineStr := engineStatus(m.engine)
	statusLine := fmt.Sprintf("  ENGINE: %s  |  %s  |  ●%d flows  |  ↑%s pkt/s  |  Tab:panels  [M]ap  [F]ile  q:quit",
		engineStr,
		throughput,
		activeCount,
		formatCount(pktsPerSec))
	content += "\n" + lipgloss.NewStyle().Foreground(colorAccent).Render(statusLine)

	return barStyle.Render(content)
}

// formatThroughput converts bits per second to a human-readable string.
func formatThroughput(bps uint64) string {
	switch {
	case bps >= 1_000_000_000:
		return fmt.Sprintf("↑%.1fGbps", float64(bps)/1_000_000_000)
	case bps >= 1_000_000:
		return fmt.Sprintf("↑%.1fMbps", float64(bps)/1_000_000)
	case bps >= 1_000:
		return fmt.Sprintf("↑%.1fKbps", float64(bps)/1_000)
	default:
		return fmt.Sprintf("↑%dbps", bps)
	}
}

// engineStatus returns "RUNNING" or "OFF" depending on engine state.
func engineStatus(eng *engine.Engine) string {
	if eng != nil && eng.Running() {
		return "RUNNING"
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

	// Rebuild flows from panel state (now includes AppID).
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
			AppID:       fd.AppID,
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
