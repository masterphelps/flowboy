// internal/tui/machines.go
package tui

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/masterphelps/flowboy/internal/config"
)

// machineMode describes what the panel is currently doing.
type machineMode int

const (
	machineNormal  machineMode = iota
	machineAdding              // inline form for new machine
	machineEditing             // inline form for editing selected machine
	machineDeleting            // "Are you sure? y/n" prompt
)

// MachineSelectedMsg is sent when the user moves the cursor to a new machine.
type MachineSelectedMsg struct {
	Machine *config.Machine
}

// MachinePanel is a sub-model that renders the machine list and handles
// CRUD operations via inline forms.
type MachinePanel struct {
	machines  []config.Machine
	cursor    int
	mode      machineMode
	nameInput textinput.Model
	ipInput   textinput.Model
	maskInput textinput.Model
	formFocus int // 0=name, 1=ip, 2=mask
	editName  string // original name when editing
	width     int
	height    int
}

// NewMachinePanel returns an initialised MachinePanel.
func NewMachinePanel() MachinePanel {
	ni := textinput.New()
	ni.Placeholder = "hostname"
	ni.CharLimit = 40
	ni.Width = 20

	ii := textinput.New()
	ii.Placeholder = "192.168.1.1"
	ii.CharLimit = 15
	ii.Width = 15

	mi := textinput.New()
	mi.Placeholder = "24"
	mi.CharLimit = 2
	mi.Width = 4

	return MachinePanel{
		nameInput: ni,
		ipInput:   ii,
		maskInput: mi,
	}
}

// SetMachines replaces the machine list and clamps the cursor.
func (p *MachinePanel) SetMachines(machines []config.Machine) {
	p.machines = machines
	if p.cursor >= len(p.machines) {
		p.cursor = max(0, len(p.machines)-1)
	}
}

// SetSize updates the available width and height.
func (p *MachinePanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// SelectedMachine returns the currently highlighted machine, or nil.
func (p *MachinePanel) SelectedMachine() *config.Machine {
	if len(p.machines) == 0 || p.cursor >= len(p.machines) {
		return nil
	}
	m := p.machines[p.cursor]
	return &m
}

// Update handles key messages when the panel is focused.
// It returns an optional command (e.g. a MachineSelectedMsg).
func (p *MachinePanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch p.mode {
		case machineNormal:
			return p.updateNormal(msg)
		case machineAdding, machineEditing:
			return p.updateForm(msg)
		case machineDeleting:
			return p.updateDelete(msg)
		}
	}
	return nil
}

// -- Normal mode --------------------------------------------------------

func (p *MachinePanel) updateNormal(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		if p.cursor < len(p.machines)-1 {
			p.cursor++
			return p.selectionCmd()
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
			return p.selectionCmd()
		}
	case "n":
		p.enterAddMode()
	case "e":
		if len(p.machines) > 0 {
			p.enterEditMode()
		}
	case "d":
		if len(p.machines) > 0 {
			p.mode = machineDeleting
		}
	}
	return nil
}

func (p *MachinePanel) enterAddMode() {
	p.mode = machineAdding
	p.formFocus = 0
	p.nameInput.SetValue("")
	p.ipInput.SetValue("")
	p.maskInput.SetValue("24")
	p.nameInput.Focus()
	p.ipInput.Blur()
	p.maskInput.Blur()
}

func (p *MachinePanel) enterEditMode() {
	m := p.machines[p.cursor]
	p.mode = machineEditing
	p.formFocus = 0
	p.editName = m.Name
	p.nameInput.SetValue(m.Name)
	p.ipInput.SetValue(m.IP.String())
	ones, _ := m.Mask.Size()
	p.maskInput.SetValue(strconv.Itoa(ones))
	p.nameInput.Focus()
	p.ipInput.Blur()
	p.maskInput.Blur()
}

// -- Form mode (add / edit) ---------------------------------------------

func (p *MachinePanel) updateForm(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.mode = machineNormal
		return nil
	case "tab", "down":
		p.formFocus = (p.formFocus + 1) % 3
		p.focusField()
		return nil
	case "shift+tab", "up":
		p.formFocus = (p.formFocus + 2) % 3
		p.focusField()
		return nil
	case "enter":
		m, ok := p.validateForm()
		if !ok {
			return nil
		}
		if p.mode == machineAdding {
			p.machines = append(p.machines, m)
			p.cursor = len(p.machines) - 1
		} else {
			// Editing: replace in-place. Remove old entry first (name may differ).
			for i, existing := range p.machines {
				if existing.Name == p.editName {
					p.machines[i] = m
					break
				}
			}
		}
		p.mode = machineNormal
		return p.machineChangedCmd(m)
	}

	// Forward to the focused textinput.
	p.updateFocusedInput(msg)
	return nil
}

func (p *MachinePanel) focusField() {
	p.nameInput.Blur()
	p.ipInput.Blur()
	p.maskInput.Blur()
	switch p.formFocus {
	case 0:
		p.nameInput.Focus()
	case 1:
		p.ipInput.Focus()
	case 2:
		p.maskInput.Focus()
	}
}

func (p *MachinePanel) updateFocusedInput(msg tea.KeyMsg) {
	switch p.formFocus {
	case 0:
		m, _ := p.nameInput.Update(msg)
		p.nameInput = m
	case 1:
		m, _ := p.ipInput.Update(msg)
		p.ipInput = m
	case 2:
		m, _ := p.maskInput.Update(msg)
		p.maskInput = m
	}
}

func (p *MachinePanel) validateForm() (config.Machine, bool) {
	name := strings.TrimSpace(p.nameInput.Value())
	ipStr := strings.TrimSpace(p.ipInput.Value())
	maskStr := strings.TrimSpace(p.maskInput.Value())

	if name == "" || ipStr == "" || maskStr == "" {
		return config.Machine{}, false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return config.Machine{}, false
	}
	maskBits, err := strconv.Atoi(maskStr)
	if err != nil || maskBits < 0 || maskBits > 32 {
		return config.Machine{}, false
	}
	return config.Machine{
		Name: name,
		IP:   ip,
		Mask: net.CIDRMask(maskBits, 32),
	}, true
}

// -- Delete confirm mode ------------------------------------------------

func (p *MachinePanel) updateDelete(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "y":
		if p.cursor < len(p.machines) {
			removed := p.machines[p.cursor]
			p.machines = append(p.machines[:p.cursor], p.machines[p.cursor+1:]...)
			if p.cursor >= len(p.machines) && p.cursor > 0 {
				p.cursor--
			}
			p.mode = machineNormal
			return p.machineDeletedCmd(removed)
		}
		p.mode = machineNormal
	case "n", "esc":
		p.mode = machineNormal
	}
	return nil
}

// -- Commands -----------------------------------------------------------

func (p *MachinePanel) selectionCmd() tea.Cmd {
	m := p.SelectedMachine()
	return func() tea.Msg {
		return MachineSelectedMsg{Machine: m}
	}
}

// MachineChangedMsg is emitted when a machine is added or edited.
type MachineChangedMsg struct {
	Machine config.Machine
	OldName string // non-empty when editing
}

// MachineDeletedMsg is emitted when a machine is deleted.
type MachineDeletedMsg struct {
	Machine config.Machine
}

func (p *MachinePanel) machineChangedCmd(m config.Machine) tea.Cmd {
	oldName := ""
	if p.mode == machineEditing {
		oldName = p.editName
	}
	return func() tea.Msg {
		return MachineChangedMsg{Machine: m, OldName: oldName}
	}
}

func (p *MachinePanel) machineDeletedCmd(m config.Machine) tea.Cmd {
	return func() tea.Msg {
		return MachineDeletedMsg{Machine: m}
	}
}

// -- View ---------------------------------------------------------------

// View renders the machine list panel contents (without the outer border).
func (p *MachinePanel) View() string {
	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render("MACHINES"))
	b.WriteString("\n")

	if p.mode == machineAdding || p.mode == machineEditing {
		b.WriteString(p.renderForm())
	} else if p.mode == machineDeleting {
		b.WriteString(p.renderDeleteConfirm())
	} else if len(p.machines) == 0 {
		b.WriteString("  No machines configured\n")
	} else {
		b.WriteString(p.renderList())
	}

	// Pad to fill available height so footer stays at the bottom.
	lines := strings.Count(b.String(), "\n")
	// Reserve 1 line for the footer.
	remaining := p.height - lines - 2
	for i := 0; i < remaining; i++ {
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(p.renderFooter())

	return b.String()
}

func (p *MachinePanel) renderList() string {
	var b strings.Builder
	for i, m := range p.machines {
		ones, _ := m.Mask.Size()
		line := fmt.Sprintf("%-18s %s/%d", m.Name, m.IP.String(), ones)

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

func (p *MachinePanel) renderForm() string {
	var b strings.Builder
	label := "NEW MACHINE"
	if p.mode == machineEditing {
		label = "EDIT MACHINE"
	}
	b.WriteString(activeItemStyle.Render(label))
	b.WriteString("\n\n")

	fields := []struct {
		label string
		view  string
	}{
		{"Name: ", p.nameInput.View()},
		{"IP:   ", p.ipInput.View()},
		{"Mask: ", p.maskInput.View()},
	}
	for i, f := range fields {
		prefix := "  "
		if i == p.formFocus {
			prefix = "▸ "
		}
		b.WriteString(prefix + f.label + f.view + "\n")
	}
	b.WriteString("\n  Enter: save  Esc: cancel  Tab: next field\n")
	return b.String()
}

func (p *MachinePanel) renderDeleteConfirm() string {
	if p.cursor >= len(p.machines) {
		return ""
	}
	m := p.machines[p.cursor]
	var b strings.Builder
	b.WriteString(activeItemStyle.Render("DELETE MACHINE"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Remove %q?\n\n", m.Name))
	b.WriteString("  Are you sure? [y/n]\n")
	return b.String()
}

func (p *MachinePanel) renderFooter() string {
	return lipgloss.NewStyle().
		Foreground(colorAccent).
		Render("[N]ew  [E]dit  [D]elete")
}

