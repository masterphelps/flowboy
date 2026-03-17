// internal/tui/configpanel.go
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/masterphelps/flowboy/internal/config"
)

// configPanelMode describes what the config panel is currently doing.
type configPanelMode int

const (
	configNormal  configPanelMode = iota
	configSaveAs                  // text input for save-as filename
)

// ConfigPanel provides file/config management matching the web GUI's File Drawer.
// Lists config files, handles open/save/save-as/new operations.
type ConfigPanel struct {
	configDir   string
	configPath  string
	configs     []string // list of yaml filenames in the config directory
	cursor      int
	mode        configPanelMode
	width       int
	height      int
	saveAsInput textinput.Model
	statusMsg   string
}

// NewConfigPanel returns an initialised ConfigPanel.
func NewConfigPanel(configPath string) ConfigPanel {
	si := textinput.New()
	si.Placeholder = "filename.yaml"
	si.CharLimit = 60
	si.Width = 40

	cp := ConfigPanel{
		configPath: configPath,
		configDir:  filepath.Dir(configPath),
		saveAsInput: si,
	}
	cp.refreshList()
	return cp
}

// SetSize updates the available width and height.
func (p *ConfigPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// SetConfigPath updates the current config path and refreshes the file list.
func (p *ConfigPanel) SetConfigPath(path string) {
	p.configPath = path
	p.configDir = filepath.Dir(path)
	p.refreshList()
}

func (p *ConfigPanel) refreshList() {
	entries, err := os.ReadDir(p.configDir)
	if err != nil {
		p.configs = nil
		return
	}
	p.configs = nil
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			p.configs = append(p.configs, e.Name())
		}
	}
	if p.cursor >= len(p.configs) {
		p.cursor = max(0, len(p.configs)-1)
	}
}

// Update handles key messages when the config panel is active.
func (p *ConfigPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if p.mode == configSaveAs {
			return p.updateSaveAs(msg)
		}
		return p.updateNormal(msg)
	}
	return nil
}

func (p *ConfigPanel) updateNormal(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		if p.cursor < len(p.configs)-1 {
			p.cursor++
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "enter":
		// Open selected config
		if len(p.configs) > 0 {
			name := p.configs[p.cursor]
			path := filepath.Join(p.configDir, name)
			return p.openConfigCmd(path)
		}
	case "s":
		// Save current config
		return p.saveConfigCmd()
	case "a":
		// Save-as mode
		p.mode = configSaveAs
		p.saveAsInput.SetValue("")
		p.saveAsInput.Focus()
	case "n":
		// New blank config
		return p.newConfigCmd()
	}
	return nil
}

func (p *ConfigPanel) updateSaveAs(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.mode = configNormal
		return nil
	case "enter":
		name := strings.TrimSpace(p.saveAsInput.Value())
		if name == "" {
			return nil
		}
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			name += ".yaml"
		}
		path := filepath.Join(p.configDir, name)
		p.mode = configNormal
		return p.saveAsConfigCmd(path, name)
	}
	m, _ := p.saveAsInput.Update(msg)
	p.saveAsInput = m
	return nil
}

// -- Commands -----------------------------------------------------------------

// ConfigOpenMsg is emitted when a config file should be loaded.
type ConfigOpenMsg struct {
	Path string
	Cfg  *config.Config
}

// ConfigSaveMsg is emitted to save the current config.
type ConfigSaveMsg struct{}

// ConfigSaveAsMsg is emitted to save config under a new name.
type ConfigSaveAsMsg struct {
	Path string
	Name string
}

// ConfigNewMsg is emitted to create a blank config.
type ConfigNewMsg struct{}

func (p *ConfigPanel) openConfigCmd(path string) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig(path)
		if err != nil {
			return nil
		}
		return ConfigOpenMsg{Path: path, Cfg: cfg}
	}
}

func (p *ConfigPanel) saveConfigCmd() tea.Cmd {
	return func() tea.Msg {
		return ConfigSaveMsg{}
	}
}

func (p *ConfigPanel) saveAsConfigCmd(path, name string) tea.Cmd {
	return func() tea.Msg {
		return ConfigSaveAsMsg{Path: path, Name: name}
	}
}

func (p *ConfigPanel) newConfigCmd() tea.Cmd {
	return func() tea.Msg {
		return ConfigNewMsg{}
	}
}

// -- View ---------------------------------------------------------------------

// View renders the config panel.
func (p *ConfigPanel) View() string {
	var b strings.Builder

	current := filepath.Base(p.configPath)
	b.WriteString(headerStyle.Render(fmt.Sprintf("FILE — %s", current)))
	b.WriteString("\n")

	if p.mode == configSaveAs {
		b.WriteString(p.renderSaveAs())
		return b.String()
	}

	// Config file list
	if len(p.configs) == 0 {
		b.WriteString("  No config files found\n")
	} else {
		for i, name := range p.configs {
			suffix := ""
			if name == current {
				suffix = " (loaded)"
			}
			line := fmt.Sprintf("%-30s%s", name, suffix)

			if i == p.cursor && name == current {
				b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("▸ " + line))
			} else if i == p.cursor {
				b.WriteString(activeItemStyle.Render("▸ " + line))
			} else if name == current {
				b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render("  " + line))
			} else if i%2 == 0 {
				b.WriteString(dimItemStyle.Render("  " + line))
			} else {
				b.WriteString("  " + line)
			}
			b.WriteString("\n")
		}
	}

	if p.statusMsg != "" {
		b.WriteString("\n  " + p.statusMsg + "\n")
	}

	// Pad to fill height
	lines := strings.Count(b.String(), "\n")
	remaining := p.height - lines - 2
	for i := 0; i < remaining; i++ {
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorAccent).
		Render("[↵]Open  [S]ave  [A]Save-As  [N]ew  [Esc]Back"))

	return b.String()
}

func (p *ConfigPanel) renderSaveAs() string {
	var b strings.Builder
	b.WriteString(activeItemStyle.Render("SAVE AS"))
	b.WriteString("\n\n")
	b.WriteString("  Filename: " + p.saveAsInput.View() + "\n")
	b.WriteString("\n  Enter: save  Esc: cancel\n")
	return b.String()
}
