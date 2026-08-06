package workspace

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// StatusBar holds dynamic status information for the workspace footer.
type StatusBar struct {
	FocusedPane string
	ProxyStatus string
	ReconStatus string
	ScopeInfo   string
	Time        string
}

// Command identifies a workspace-level action requested by a key sequence.
type Command string

const (
	CommandSplitHorizontal Command = "split-horizontal"
	CommandSplitVertical   Command = "split-vertical"
)

// CommandMsg is emitted after a workspace command prefix is completed.
// The application supplies the concrete view to split into the layout.
type CommandMsg struct {
	Action Command
}

// AllClosedMsg is emitted when the last pane is closed, leaving no panes.
// The application should quit in response.
type AllClosedMsg struct{}

// Manager owns the layout tree and routes events to panes.
type Manager struct {
	layout           *Layout
	focusedID        string
	status           StatusBar
	width            int
	height           int
	waitingForWindow bool // Ctrl+w prefix active
}

// NewManager creates a new workspace manager.
func NewManager() *Manager {
	return &Manager{}
}

// Layout returns the root layout node.
func (m *Manager) Layout() *Layout {
	return m.layout
}

// FocusedPane returns the currently focused pane, or nil.
func (m *Manager) FocusedPane() *Pane {
	if m.layout == nil || m.focusedID == "" {
		return nil
	}
	return m.layout.FindPaneByID(m.focusedID)
}

// AddPane adds a pane as the root layout, replacing any existing layout.
func (m *Manager) AddPane(v View) *Pane {
	p := &Pane{
		ID:      v.ID(),
		View:    v,
		Focused: true,
		Width:   m.width,
		Height:  m.height,
	}
	m.layout = NewLeaf(p)
	m.focusedID = p.ID
	v.Focus()
	m.Resize(m.width, m.height)
	return p
}

// SplitHSplit splits the focused pane horizontally (left/right) with a new view.
func (m *Manager) SplitHSplit(v View) *Pane {
	return m.split(LayoutHSplit, v)
}

// SplitVSplit splits the focused pane vertically (top/bottom) with a new view.
func (m *Manager) SplitVSplit(v View) *Pane {
	return m.split(LayoutVSplit, v)
}

func (m *Manager) split(kind LayoutKind, v View) *Pane {
	if m.layout == nil {
		return m.AddPane(v)
	}

	focused := m.FocusedPane()
	if focused == nil {
		return m.AddPane(v)
	}

	newPane := &Pane{
		ID:     v.ID(),
		View:   v,
		Width:  focused.Width,
		Height: focused.Height,
	}

	// Find the leaf node containing the focused pane and replace it with a split.
	m.layout = m.replaceLeaf(m.layout, focused.ID, kind, newPane)
	m.focusedID = newPane.ID
	v.Focus()
	m.Resize(m.width, m.height)
	return newPane
}

// replaceLeaf finds the leaf with the given ID and replaces it with a split.
func (m *Manager) replaceLeaf(node *Layout, id string, kind LayoutKind, newPane *Pane) *Layout {
	if node == nil {
		return nil
	}
	if node.Kind == LayoutLeaf && node.Pane != nil && node.Pane.ID == id {
		oldPane := node.Pane
		oldPane.Focused = false
		oldPane.View.Blur()

		switch kind {
		case LayoutHSplit:
			return NewHSplit(NewLeaf(oldPane), NewLeaf(newPane), 0.5)
		case LayoutVSplit:
			return NewVSplit(NewLeaf(oldPane), NewLeaf(newPane), 0.5)
		default:
			return NewLeaf(newPane)
		}
	}
	if node.Left != nil {
		node.Left = m.replaceLeaf(node.Left, id, kind, newPane)
	}
	if node.Right != nil {
		node.Right = m.replaceLeaf(node.Right, id, kind, newPane)
	}
	return node
}

// CloseFocused removes the focused pane. If it's the last pane, the layout becomes nil.
func (m *Manager) CloseFocused() {
	if m.layout == nil || m.focusedID == "" {
		return
	}

	// If it's the only pane, clear everything.
	if len(m.layout.Panes()) <= 1 {
		m.layout = nil
		m.focusedID = ""
		return
	}

	// Find the parent of the focused pane and replace it with the sibling.
	m.layout = m.removeLeaf(m.layout, m.focusedID)
	// Focus the first remaining pane.
	panes := m.layout.Panes()
	if len(panes) > 0 {
		m.focusPane(panes[0].ID)
	}
	m.Resize(m.width, m.height)
}

// removeLeaf removes the leaf with the given ID, replacing its parent with the sibling.
func (m *Manager) removeLeaf(node *Layout, id string) *Layout {
	if node == nil {
		return nil
	}
	if node.Kind == LayoutLeaf {
		if node.Pane != nil && node.Pane.ID == id {
			return nil // This leaf is being removed.
		}
		return node
	}

	// Check if either child contains the target.
	if node.Left != nil {
		leftPanes := node.Left.Panes()
		for _, p := range leftPanes {
			if p.ID == id {
				// Replace parent with the right child.
				return node.Right
			}
		}
	}
	if node.Right != nil {
		rightPanes := node.Right.Panes()
		for _, p := range rightPanes {
			if p.ID == id {
				return node.Left
			}
		}
	}

	// Recurse.
	if node.Left != nil {
		node.Left = m.removeLeaf(node.Left, id)
	}
	if node.Right != nil {
		node.Right = m.removeLeaf(node.Right, id)
	}
	return node
}

// CloseAllButFocused removes all panes except the focused one.
func (m *Manager) CloseAllButFocused() {
	if m.layout == nil || m.focusedID == "" {
		return
	}
	focused := m.FocusedPane()
	if focused == nil {
		return
	}
	m.layout = NewLeaf(focused)
	focused.Focused = true
	m.Resize(m.width, m.height)
}

// FocusNext moves focus to the next pane in the layout.
func (m *Manager) FocusNext() {
	panes := m.layout.Panes()
	if len(panes) < 2 {
		return
	}
	for i, p := range panes {
		if p.ID == m.focusedID {
			next := (i + 1) % len(panes)
			m.focusPane(panes[next].ID)
			return
		}
	}
	// Fallback: focus first.
	m.focusPane(panes[0].ID)
}

// FocusPrev moves focus to the previous pane.
func (m *Manager) FocusPrev() {
	panes := m.layout.Panes()
	if len(panes) < 2 {
		return
	}
	for i, p := range panes {
		if p.ID == m.focusedID {
			prev := (i - 1 + len(panes)) % len(panes)
			m.focusPane(panes[prev].ID)
			return
		}
	}
	m.focusPane(panes[0].ID)
}

// FocusLeft moves focus to the pane to the left (in a horizontal split).
func (m *Manager) FocusLeft() {
	m.FocusPrev()
}

// FocusRight moves focus to the pane to the right.
func (m *Manager) FocusRight() {
	m.FocusNext()
}

// FocusUp moves focus up.
func (m *Manager) FocusUp() {
	m.FocusPrev()
}

// FocusDown moves focus down.
func (m *Manager) FocusDown() {
	m.FocusNext()
}

// Equalize resets all split weights to 0.5.
func (m *Manager) Equalize() {
	m.equalize(m.layout)
	m.Resize(m.width, m.height)
}

func (m *Manager) equalize(node *Layout) {
	if node == nil {
		return
	}
	if node.Kind == LayoutHSplit || node.Kind == LayoutVSplit {
		node.Weight = 0.5
		m.equalize(node.Left)
		m.equalize(node.Right)
	}
}

// SetStatus updates the status bar.
func (m *Manager) SetStatus(s StatusBar) {
	m.status = s
}

// Update dispatches a message to all panes and handles keyboard routing.
func (m *Manager) Update(msg tea.Msg) tea.Cmd {
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKeyPress(v)
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.Resize(m.width, m.height)
		return nil
	default:
		// Dispatch to all panes.
		return m.dispatchToAll(msg)
	}
}

func (m *Manager) handleKeyPress(v tea.KeyPressMsg) tea.Cmd {
	// Ctrl+w prefix handling.
	if m.waitingForWindow {
		m.waitingForWindow = false
		switch {
		case key.Matches(v, key.NewBinding(key.WithKeys("s"))):
			return func() tea.Msg {
				return CommandMsg{Action: CommandSplitHorizontal}
			}
		case key.Matches(v, key.NewBinding(key.WithKeys("v"))):
			return func() tea.Msg {
				return CommandMsg{Action: CommandSplitVertical}
			}
		case key.Matches(v, key.NewBinding(key.WithKeys("c"))):
			m.CloseFocused()
			if m.layout == nil {
				return func() tea.Msg { return AllClosedMsg{} }
			}
			return nil
		case key.Matches(v, key.NewBinding(key.WithKeys("o"))):
			m.CloseAllButFocused()
			return nil
		case key.Matches(v, key.NewBinding(key.WithKeys("="))):
			m.Equalize()
			return nil
		default:
			// Pass through to the focused pane.
		}
	}

	// Check for Ctrl+w prefix.
	if key.Matches(v, key.NewBinding(key.WithKeys("ctrl+w"))) {
		m.waitingForWindow = true
		return nil
	}

	// Directional focus movement.
	switch {
	case key.Matches(v, key.NewBinding(key.WithKeys("ctrl+h"))):
		m.FocusLeft()
		return nil
	case key.Matches(v, key.NewBinding(key.WithKeys("ctrl+l"))):
		m.FocusRight()
		return nil
	case key.Matches(v, key.NewBinding(key.WithKeys("ctrl+j"))):
		m.FocusDown()
		return nil
	case key.Matches(v, key.NewBinding(key.WithKeys("ctrl+k"))):
		m.FocusUp()
		return nil
	}

	// Route to focused pane.
	focused := m.FocusedPane()
	if focused != nil {
		updated, cmd := focused.View.Update(v)
		focused.View = updated
		return cmd
	}
	return nil
}

func (m *Manager) dispatchToAll(msg tea.Msg) tea.Cmd {
	if m.layout == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, p := range m.layout.Panes() {
		updated, cmd := p.View.Update(msg)
		p.View = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

// Resize recalculates all pane dimensions.
func (m *Manager) Resize(width, height int) {
	m.width = width
	m.height = height
	if m.layout != nil {
		// Reserve 2 lines: help bar + status bar.
		m.layout.Resize(width, height-2)
	}
}

// View renders the full workspace including help bar and status bar.
func (m *Manager) View() string {
	if m.layout == nil {
		return ""
	}

	content := m.layout.Render()

	focused := m.FocusedPane()

	// Help bar — shows the focused pane's keybindings plus workspace-global keys.
	helpText := ""
	if focused != nil {
		helpText = focused.View.HelpText()
	}
	workspaceKeys := "^h/j/k/l: focus  ^w: split  0: hist  4: scope  5: recon  ::: cmd"
	if helpText != "" {
		helpText += "  "
	}
	helpText += workspaceKeys
	helpBar := lipgloss.NewStyle().
		Width(m.width).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("250")).
		Render(helpText)

	// Status bar.
	focusedTitle := ""
	if focused != nil {
		focusedTitle = focused.View.Title()
	}
	statusText := fmt.Sprintf(" %s | proxy: %s | scope: %s | %s",
		focusedTitle,
		m.status.ProxyStatus,
		m.status.ScopeInfo,
		time.Now().Format("15:04:05"),
	)
	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("250")).
		Render(statusText)

	return content + "\n" + helpBar + "\n" + statusBar
}

// focusPane sets focus to the pane with the given ID.
func (m *Manager) focusPane(id string) {
	for _, p := range m.layout.Panes() {
		if p.ID == id {
			p.Focused = true
			p.View.Focus()
		} else if p.Focused {
			p.Focused = false
			p.View.Blur()
		}
	}
	m.focusedID = id
	m.status.FocusedPane = id
}

// FocusPane sets focus to the pane with the given ID (public).
func (m *Manager) FocusPane(id string) {
	m.focusPane(id)
}
