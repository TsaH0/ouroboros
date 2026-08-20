package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/TsaH0/ouroboros/internal/intercept"
	"github.com/TsaH0/ouroboros/internal/model"
	"github.com/TsaH0/ouroboros/internal/msg"
	"github.com/TsaH0/ouroboros/internal/project"
	"github.com/TsaH0/ouroboros/internal/proxy"
	"github.com/TsaH0/ouroboros/internal/recon"
	"github.com/TsaH0/ouroboros/internal/repeater"
	"github.com/TsaH0/ouroboros/internal/scope"
	"github.com/TsaH0/ouroboros/internal/store"
	"github.com/TsaH0/ouroboros/internal/workspace"
)

// AppModel is the top-level Bubble Tea model for the Ouroboros TUI.
type AppModel struct {
	store                  store.Store
	proxy                  *proxy.Proxy
	repeaterSvc            repeater.Service
	reconMgr               *recon.Engine
	reconProgressListening bool
	scopeMgr               *scope.Manager
	ws                     *workspace.Manager
	history                *HistoryModel
	help                   help.Model
	quitting               bool
	interceptEnabled       bool
	width                  int
	height                 int
	ready                  bool
	viewSeq                uint64
	commandMode            bool
	commandInput           textinput.Model
	projectStore           *project.Store
	activeProject          string
	// Project picker overlay (N: new, P: switch).
	projectPicker    bool
	projectList      []string
	projectCursor    int
	creatingProject  bool
	newProjectInput  textinput.Model
	// Confirm wipe (D: delete persisted history).
	confirmWipe bool
	// Intercept queue tab (3) and floating editor.
	interceptModel *InterceptModel
	floating       workspace.View
}

// SetReconEngine sets the recon engine (called from main).
func (m *AppModel) SetReconEngine(engine *recon.Engine) {
	m.reconMgr = engine
}

func (m *AppModel) Init() tea.Cmd {
	return nil
}

func waitForReconProgress(ch <-chan recon.ProgressUpdate) tea.Cmd {
	return func() tea.Msg {
		select {
		case update, ok := <-ch:
			if !ok {
				return nil
			}
			return update
		}
	}
}

func (m *AppModel) Update(mgs tea.Msg) (tea.Model, tea.Cmd) {
	// Handle application-level messages first.
	switch v := mgs.(type) {
	case tea.PasteMsg:
		// Bracketed paste — route to focused input (scope, detail, repeater, command, new-project)
		if m.floating != nil {
			var cmd tea.Cmd
			m.floating, cmd = m.floating.Update(mgs)
			return m, cmd
		}
		if m.creatingProject {
			var cmd tea.Cmd
			m.newProjectInput, cmd = m.newProjectInput.Update(mgs)
			return m, cmd
		}
		if m.commandMode {
			var cmd tea.Cmd
			m.commandInput, cmd = m.commandInput.Update(mgs)
			return m, cmd
		}
		if m.ws != nil {
			cmd := m.ws.Update(mgs)
			return m, cmd
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.ready = true
		m.help.SetWidth(v.Width)
		if m.ws != nil {
			m.ws.Update(v)
		}
		return m, nil
	case tea.KeyPressMsg:
		// Ctrl+c is the only unconditional quit key.
		if key.Matches(v, key.NewBinding(key.WithKeys("ctrl+c"))) {
			m.quitting = true
			return m, tea.Quit
		}

		// Confirm wipe overlay takes priority.
		if m.confirmWipe {
			return m.handleConfirmWipeKey(v)
		}

		// Floating editor (detail/repeater for intercept) takes priority over panes.
		if m.floating != nil {
			return m.handleFloatingKey(v)
		}

		// Project picker / new-project overlays take priority.
		if m.projectPicker {
			return m.handleProjectPickerKey(v)
		}
		if m.creatingProject {
			return m.handleNewProjectKey(v)
		}

		// Command mode: route all keys to the command input.
		if m.commandMode {
			return m.handleCommandKey(v)
		}

		// Enter command mode with ':' (unless focused pane is editing).
		if key.Matches(v, key.NewBinding(key.WithKeys(":"))) {
			focused := m.ws.FocusedPane()
			if focused == nil || !focused.View.IsEditing() {
				m.commandMode = true
				m.commandInput = textinput.New()
				m.commandInput.Prompt = ""
				m.commandInput.Focus()
				return m, textinput.Blink
			}
		}

		// Global view-opening shortcuts (0/4/5/i) work from any pane
		// unless the focused pane has a text input capturing keystrokes.
		if handled, cmd := m.handleGlobalKey(v); handled {
			return m, cmd
		}
		if handled, cmd := m.handleHistoryKey(v); handled {
			return m, cmd
		}
		if handled, cmd := m.handleInterceptKey(v); handled {
			return m, cmd
		}
		// Delegate all other keys to the workspace manager.
		if m.ws != nil {
			cmd := m.ws.Update(v)
			return m, cmd
		}
		return m, nil

	case workspace.AllClosedMsg:
		m.quitting = true
		return m, tea.Quit

	case backToListMsg:
		if m.floating != nil {
			m.closeFloating()
			return m, nil
		}
		if m.ws != nil {
			m.ws.CloseFocused()
			if m.ws.Layout() == nil {
				m.quitting = true
				return m, tea.Quit
			}
		}
		return m, nil

	case workspace.CommandMsg:
		return m, m.handleWorkspaceCommand(v)

	case msg.FlowCompleted:
		// Dispatch to all panes (history, etc.).
		if m.ws != nil {
			return m, m.ws.Update(v)
		}
		return m, nil

	case msg.ScopePresetChangedMsg:
		// Broadcast preset change to all history panes so they re-filter.
		if m.ws != nil {
			return m, m.ws.Update(v)
		}
		return m, nil

	case msg.InterceptionRequired:
		flow, err := m.store.GetFlow(context.Background(), v.FlowID)
		if err != nil || flow == nil {
			return m, nil
		}
		// Queue into intercept tab instead of auto-piling panes.
		m.ensureInterceptModel()
		m.interceptModel.AddFlow(flow)
		// Ensure intercept pane exists (auto-open on first intercept).
		hasInterceptPane := false
		if m.ws != nil {
			for _, p := range m.ws.Layout().Panes() {
				if _, ok := p.View.(*InterceptModel); ok {
					hasInterceptPane = true
					break
				}
			}
			if !hasInterceptPane {
				// Open intercept queue as a new pane (non-intrusive).
				pane := m.ws.SplitHSplit(m.interceptModel)
				return m, pane.View.Init()
			}
		}
		// Also broadcast to intercept pane if already open.
		if m.ws != nil {
			return m, m.ws.Update(v)
		}
		return m, nil

	case msg.ForwardInterceptedFlow:
		if m.proxy != nil {
			m.proxy.HandleInterceptCommand(v)
		}
		if m.interceptModel != nil {
			m.interceptModel.RemoveFlow(v.FlowID)
		}
		if m.floating != nil {
			// If floating detail was for this flow, close it.
			if dv, ok := m.floating.(*detailView); ok && dv.flow != nil && dv.flow.ID == v.FlowID {
				m.closeFloating()
			} else if v.FlowID == "" {
				m.closeFloating()
			}
		}
		// Also close detail pane if it was a split pane for this flow.
		if m.ws != nil {
			for _, p := range m.ws.Layout().Panes() {
				if dv, ok := p.View.(*detailView); ok && dv.flow != nil && dv.flow.ID == v.FlowID {
					m.ws.CloseFocused()
					break
				}
			}
		}
		return m, nil

	case msg.DropInterceptedFlow:
		if m.proxy != nil {
			m.proxy.HandleInterceptCommandDrop(v)
		}
		if m.interceptModel != nil {
			m.interceptModel.RemoveFlow(v.FlowID)
		}
		if m.floating != nil {
			if dv, ok := m.floating.(*detailView); ok && dv.flow != nil && dv.flow.ID == v.FlowID {
				m.closeFloating()
			} else if v.FlowID == "" {
				m.closeFloating()
			}
		}
		if m.ws != nil {
			for _, p := range m.ws.Layout().Panes() {
				if dv, ok := p.View.(*detailView); ok && dv.flow != nil && dv.flow.ID == v.FlowID {
					m.ws.CloseFocused()
					break
				}
			}
		}
		return m, nil

	case detailOpenRepeaterMsg:
		repeater := NewRepeaterModel(v.flow, m.width, m.height)
		rv := &repeaterView{RepeaterModel: &repeater, id: m.nextViewID("repeater")}
		if m.floating != nil {
			// Detail was floating (intercept queue) — replace with repeater floating
			return m, m.openFloating(rv)
		}
		pane := m.ws.SplitHSplit(rv)
		return m, pane.View.Init()

	case recon.ProgressUpdate:
		if m.ws != nil {
			return m, m.ws.Update(v)
		}
		return m, nil

	case reconResultMsg:
		if m.ws != nil {
			return m, m.ws.Update(v)
		}
		return m, nil

	case reconRunMsg:
		if m.reconMgr == nil {
			return m, func() tea.Msg {
				return reconResultMsg{err: fmt.Errorf("recon engine is not configured")}
			}
		}
		run := func() tea.Msg {
			summary, err := m.reconMgr.Run(context.Background(), v.target)
			return reconResultMsg{summary: summary, err: err}
		}
		if !m.reconProgressListening {
			m.reconProgressListening = true
			return m, tea.Batch(run, waitForReconProgress(m.reconMgr.ProgressChan()))
		}
		return m, run

	case repeaterResultMsg:
		if m.ws != nil {
			return m, m.ws.Update(v)
		}
		return m, nil

	case repeaterScopeBlockMsg:
		if m.ws != nil {
			return m, m.ws.Update(v)
		}
		return m, nil

	case repeaterSendMsg:
		// If this flow is currently intercepted and paused, forward the edited
		// request through the intercept channel instead of a separate replay.
		if m.proxy != nil && m.proxy.IsInterceptPending(v.flow.ID) {
			m.proxy.HandleInterceptCommand(msg.ForwardInterceptedFlow{
				FlowID: v.flow.ID,
				Edited: &msg.EditedRequest{
					Method:  v.edits.Method,
					URL:     v.edits.URL,
					Headers: v.edits.Headers,
					Body:    v.edits.Body,
				},
			})
			// Close the repeater pane that was editing the intercepted flow.
			if m.ws != nil {
				m.ws.CloseFocused()
			}
			return m, nil
		}
		return m, func() tea.Msg {
			resp, err := m.repeaterSvc.Replay(context.Background(), v.flow, v.edits)
			if errors.Is(err, repeater.ErrOutOfScope) {
				return repeaterScopeBlockMsg{}
			}
			return repeaterResultMsg{resp: resp, err: err}
		}
	}

	// Delegate remaining messages to workspace manager.
	if m.ws != nil {
		cmd := m.ws.Update(mgs)
		return m, cmd
	}
	return m, nil
}

func (m *AppModel) setInterceptEnabled(on bool) {
	m.interceptEnabled = on
	if m.proxy == nil {
		return
	}
	if on {
		m.proxy.SetInterceptService(intercept.NewMatcher([]intercept.Rule{
			{Allow: true, Host: regexp.MustCompile(".*")},
		}))
	} else {
		m.proxy.SetInterceptService(intercept.NewMatcher(nil))
	}
}

func (m *AppModel) ensureInterceptModel() {
	if m.interceptModel == nil {
		m.interceptModel = NewInterceptModel(m.store, m.width, m.height)
	}
}

func (m *AppModel) openInterceptPane() tea.Cmd {
	m.ensureInterceptModel()
	// Focus existing or create new.
	if m.ws != nil {
		for _, p := range m.ws.Layout().Panes() {
			if _, ok := p.View.(*InterceptModel); ok {
				m.ws.FocusPane(p.ID)
				return nil
			}
		}
		pane := m.ws.SplitHSplit(m.interceptModel)
		return pane.View.Init()
	}
	return nil
}

func (m *AppModel) openFloating(v workspace.View) tea.Cmd {
	m.floating = v
	if v != nil {
		v.Focus()
		// Ensure floating gets current size (centered overlay will compute itself, but view needs size).
		fw := int(float64(m.width) * 0.84)
		fh := int(float64(m.height) * 0.86)
		if fw < 40 {
			fw = m.width - 4
		}
		if fh < 10 {
			fh = m.height - 4
		}
		v.Resize(fw, fh)
		return v.Init()
	}
	return nil
}

func (m *AppModel) closeFloating() {
	if m.floating != nil {
		m.floating.Blur()
		m.floating = nil
	}
}

func (m *AppModel) handleFloatingKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.floating == nil {
		return m, nil
	}
	// esc/q closes floating without forwarding (flow stays paused until f/d).
	if key.Matches(v, key.NewBinding(key.WithKeys("esc", "q"))) {
		// If floating is an intercept detail, q should just close overlay, not drop.
		// User can still f/d inside detail. So esc/q closes overlay only.
		if _, ok := m.floating.(*detailView); ok {
			// Check if user was editing — let detail handle esc first
			updated, cmd := m.floating.Update(v)
			if cmd != nil {
				m.floating = updated
				return m, cmd
			}
			// If detail didn't consume, close floating
			m.closeFloating()
			return m, nil
		}
		m.closeFloating()
		return m, nil
	}
	updated, cmd := m.floating.Update(v)
	m.floating = updated
	// If the floating detail emitted a Forward/Drop message, it will be handled
	// as a tea.Msg in the next Update cycle. For direct cmd returns (like backToListMsg
	// from detail's q), we need to catch it: check if update produced a command that
	// sends backToListMsg — simplest is to let next Update handle, but we also
	// handle immediate close if floating requested back.
	if cmd != nil {
		// Peek: if the floating view wants to close (backToList), close overlay.
		// We can't peek into cmd, so let it flow; but also handle esc above.
		return m, cmd
	}
	return m, nil
}

func (m *AppModel) handleInterceptKey(v tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.ws == nil {
		return false, nil
	}
	focused := m.ws.FocusedPane()
	if focused == nil {
		return false, nil
	}
	im, ok := focused.View.(*InterceptModel)
	if !ok {
		return false, nil
	}
	if m.ws.WaitingForWindow() {
		return false, nil
	}
	switch {
	case key.Matches(v, key.NewBinding(key.WithKeys("enter"))):
		flow := im.SelectedFlow()
		if flow == nil {
			return true, nil
		}
		// Open floating editable detail for the selected intercepted flow.
		detail := NewDetailModel(flow, m.width, m.height)
		return true, m.openFloating(&detailView{DetailModel: &detail, id: m.nextViewID("detail-float")})
	case key.Matches(v, key.NewBinding(key.WithKeys("f"))):
		flow := im.SelectedFlow()
		if flow == nil {
			return true, nil
		}
		// Forward directly from queue without opening editor.
		if m.proxy != nil {
			m.proxy.HandleInterceptCommand(msg.ForwardInterceptedFlow{FlowID: flow.ID})
		}
		im.RemoveFlow(flow.ID)
		return true, nil
	case key.Matches(v, key.NewBinding(key.WithKeys("d"))):
		flow := im.SelectedFlow()
		if flow == nil {
			return true, nil
		}
		if m.proxy != nil {
			m.proxy.HandleInterceptCommandDrop(msg.DropInterceptedFlow{FlowID: flow.ID})
		}
		im.RemoveFlow(flow.ID)
		return true, nil
	case key.Matches(v, key.NewBinding(key.WithKeys("C"))):
		// Clear all: drop all pending.
		for _, f := range im.flows {
			if m.proxy != nil {
				m.proxy.HandleInterceptCommandDrop(msg.DropInterceptedFlow{FlowID: f.ID})
			}
		}
		im.ClearAll()
		return true, nil
	case key.Matches(v, key.NewBinding(key.WithKeys("q"))):
		// q in intercept tab closes pane (not quit).
		m.ws.CloseFocused()
		return true, nil
	}
	return false, nil
}

func (m *AppModel) renderFloatingOverlay(base string) string {
	if m.floating == nil {
		return base
	}
	fw := int(float64(m.width) * 0.84)
	fh := int(float64(m.height) * 0.86)
	if fw < 40 {
		fw = m.width - 4
	}
	if fh < 10 {
		fh = m.height - 4
	}
	// Render floating content sized to fw/fh with a border and shadow.
	content := m.floating.View()
	// Clamp floating content to fw/fh
	box := lipgloss.NewStyle().
		Width(fw).
		Height(fh).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colAccent)).
		Background(lipgloss.Color("236")).
		Padding(1, 2).
		Render(content)
	// Center over base using Place
	overlay := lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, box)
	// Dim background by overlaying — simple: just place foreground over background lines.
	// We overlay by stacking: background + overlay with transparency via Place's whitespace.
	// For terminal, we just return overlay centered; the base is underneath but Place already handles.
	// Combine: place overlay on top of base content area.
	bLines := strings.Split(base, "\n")
	oLines := strings.Split(overlay, "\n")
	// Overlay oLines onto bLines where oLines is not just spaces.
	maxH := max(len(bLines), len(oLines))
	var out []string
	for i := 0; i < maxH; i++ {
		var bl, ol string
		if i < len(bLines) {
			bl = bLines[i]
		}
		if i < len(oLines) {
			ol = oLines[i]
		}
		if strings.TrimSpace(ol) == "" {
			out = append(out, bl)
		} else {
			out = append(out, ol)
		}
	}
	return strings.Join(out, "\n")
}

// handleGlobalKey processes view-opening shortcuts (0/3/4/5) that work from
// any pane unless the focused pane is actively capturing text input.
func (m *AppModel) handleGlobalKey(v tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.ws == nil {
		return false, nil
	}
	focused := m.ws.FocusedPane()
	if focused == nil {
		return false, nil
	}
	// Suppress global shortcuts while a pane has a text input active.
	if focused.View.IsEditing() {
		return false, nil
	}
	switch {
	case key.Matches(v, key.NewBinding(key.WithKeys("0"))):
		return true, m.openHistoryPane()
	case key.Matches(v, key.NewBinding(key.WithKeys("4"))):
		return true, m.openScopePane()
	case key.Matches(v, key.NewBinding(key.WithKeys("5"))):
		return true, m.openReconPane()
	case key.Matches(v, key.NewBinding(key.WithKeys("i"))):
		return m.importSelectedFlowAsScope()
	case key.Matches(v, key.NewBinding(key.WithKeys("N"))):
		// N: new project — prompt for name and save current scope rules.
		m.ensureProjectStore()
		m.creatingProject = true
		m.newProjectInput = textinput.New()
		m.newProjectInput.Prompt = ""
		m.newProjectInput.Placeholder = "project name"
		m.newProjectInput.Focus()
		return true, textinput.Blink
	case key.Matches(v, key.NewBinding(key.WithKeys("P"))):
		// P: project switcher picker.
		m.ensureProjectStore()
		m.openProjectPicker()
		return true, nil
	case key.Matches(v, key.NewBinding(key.WithKeys("I"))):
		// I: toggle intercept (pause matching flows for forward/drop/edit).
		m.setInterceptEnabled(!m.interceptEnabled)
		return true, nil
	case key.Matches(v, key.NewBinding(key.WithKeys("3"))):
		return true, m.openInterceptPane()
	}
	return false, nil
}

// importSelectedFlowAsScope finds a history pane, gets its selected flow,
// adds a scope rule for that flow's host, and focuses the scope pane.
func (m *AppModel) importSelectedFlowAsScope() (bool, tea.Cmd) {
	if m.ws == nil || m.scopeMgr == nil {
		return false, nil
	}
	// Find a history pane in the workspace.
	var history *HistoryModel
	for _, p := range m.ws.Layout().Panes() {
		if h, ok := p.View.(*HistoryModel); ok {
			history = h
			break
		}
	}
	if history == nil {
		return false, nil
	}
	flow := history.SelectedFlow()
	if flow == nil || flow.Host == "" {
		return false, nil
	}
	// Strip port from host — scope rules match hostname only.
	host := flow.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	_, err := m.scopeMgr.AddRuleInMemory(scope.Rule{
		Kind:      scope.RuleKindHost,
		Pattern:   host,
		MatchMode: scope.MatchModeLiteral,
		Action:    scope.ActionInclude,
		Enabled:   true,
		Priority:  10,
	})
	if err != nil {
		return false, nil
	}
	// Focus existing scope pane, or open new one if none exists.
	return true, m.focusOrOpenScopePane()
}

// focusOrOpenScopePane focuses an existing scope pane or opens a new one.
func (m *AppModel) focusOrOpenScopePane() tea.Cmd {
	for _, p := range m.ws.Layout().Panes() {
		if _, ok := p.View.(*scopeView); ok {
			m.ws.FocusPane(p.ID)
			return nil
		}
	}
	return m.openScopePane()
}

func (m *AppModel) openHistoryPane() tea.Cmd {
	history := NewHistoryModel(m.store, m.scopeMgr, m.width, m.height)
	history.id = m.nextViewID("history")
	pane := m.ws.SplitHSplit(history)
	return pane.View.Init()
}

// handleCommandKey routes keystrokes to the command bar input.
func (m *AppModel) handleCommandKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(v, key.NewBinding(key.WithKeys("esc"))):
		m.commandMode = false
		m.commandInput.Blur()
		return m, nil

	case key.Matches(v, key.NewBinding(key.WithKeys("enter"))):
		cmd := m.commandInput.Value()
		m.commandMode = false
		m.commandInput.Blur()
		return m.executeCommand(cmd)

	default:
		var cmd tea.Cmd
		m.commandInput, cmd = m.commandInput.Update(v)
		return m, cmd
	}
}

// executeCommand parses and runs a vim-style command.
// Supported:
//
//	:w <name>   — save current scope rules as project <name>
//	:e <name>   — load project <name> into the scope manager
//	:ls         — list saved projects
//	:q          — quit
func (m *AppModel) executeCommand(input string) (tea.Model, tea.Cmd) {
	input = strings.TrimSpace(input)
	if input == "" {
		return m, nil
	}

	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "w":
		name := ""
		if len(parts) > 1 {
			name = parts[1]
		}
		if name == "" {
			name = m.activeProject
		}
		if name == "" {
			name = "default"
		}
		if m.projectStore == nil {
			ps, err := project.NewStore("")
			if err != nil {
				return m, nil
			}
			m.projectStore = ps
		}
		rules := m.scopeMgr.Rules()
		if err := m.projectStore.Save(name, rules); err != nil {
			return m, nil
		}
		m.activeProject = name
		return m, nil

	case "e":
		if len(parts) < 2 {
			return m, nil
		}
		name := parts[1]
		if m.projectStore == nil {
			ps, err := project.NewStore("")
			if err != nil {
				return m, nil
			}
			m.projectStore = ps
		}
		rules, err := m.projectStore.Load(name)
		if err != nil {
			return m, nil
		}
		m.scopeMgr.ReplaceRules(rules)
		m.activeProject = name
		// Refresh any open scope panes.
		for _, p := range m.ws.Layout().Panes() {
			if sv, ok := p.View.(*scopeView); ok {
				sv.ScopeModel.rules = m.scopeMgr.Rules()
				sv.ScopeModel.refreshTable()
			}
		}
		return m, nil

	case "ls":
		if m.projectStore == nil {
			ps, err := project.NewStore("")
			if err != nil {
				return m, nil
			}
			m.projectStore = ps
		}
		names, err := m.projectStore.List()
		if err != nil || len(names) == 0 {
			return m, nil
		}
		// Display projects by setting activeProject to the list.
		m.activeProject = strings.Join(names, ", ")
		return m, nil

	case "clear":
		// :clear — clear screen only (DB kept, restart restores).
		m.clearAllHistoryScreens()
		return m, nil
	case "clear!", "wipe", "purge":
		// :clear! / :wipe / :purge — delete persisted flows (DB + screen).
		m.confirmWipe = true
		return m, nil

	case "q":
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

// handleHistoryKey handles application-level shortcuts that are only valid
// while the history pane is focused.
func (m *AppModel) handleHistoryKey(v tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.ws == nil {
		return false, nil
	}
	focused := m.ws.FocusedPane()
	if focused == nil {
		return false, nil
	}
	if _, ok := focused.View.(*HistoryModel); !ok {
		return false, nil
	}
	// Skip history-specific keys when a Ctrl+w prefix is active.
	if m.ws.WaitingForWindow() {
		return false, nil
	}

	switch {
	case key.Matches(v, key.NewBinding(key.WithKeys("q"))):
		m.quitting = true
		return true, tea.Quit
	case key.Matches(v, key.NewBinding(key.WithKeys("enter"))):
		// Floating detail (centered modal) instead of split — avoids pane pile-up
		return m.openSelectedFlow(func(flow *model.Flow) tea.Cmd {
			detail := NewDetailModel(flow, m.width, m.height)
			return m.openFloating(&detailView{
				DetailModel: &detail,
				id:          m.nextViewID("detail"),
			})
		})
	case key.Matches(v, key.NewBinding(key.WithKeys("r"))):
		return m.openSelectedFlow(func(flow *model.Flow) tea.Cmd {
			repeater := NewRepeaterModel(flow, m.width, m.height)
			return m.openFloating(&repeaterView{
				RepeaterModel: &repeater,
				id:            m.nextViewID("repeater"),
			})
		})
	case key.Matches(v, key.NewBinding(key.WithKeys("s"))):
		// Toggle the selected flow's host scope from history.
		focused := m.ws.FocusedPane()
		history, ok := focused.View.(*HistoryModel)
		if !ok {
			return false, nil
		}
		flow := history.SelectedFlow()
		if flow == nil || flow.Host == "" {
			return true, nil
		}
		// Strip port from host — scope rules match hostname only.
		host := flow.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		// Check current status BEFORE removing old rules.
		status := m.scopeMgr.HostStatus(host)
		// Remove any existing literal host rules for this host
		// so the toggle is clean (no conflicting duplicates).
		// Use in-memory methods so rules don't persist to the DB.
		m.scopeMgr.RemoveHostRulesInMemory(host)

		if status == model.ScopeInScope {
			// Was in scope → add explicit exclude (session only).
			_, _ = m.scopeMgr.AddRuleInMemory(scope.Rule{
				Kind: scope.RuleKindHost, Pattern: host,
				MatchMode: scope.MatchModeLiteral, Action: scope.ActionExclude,
				Enabled: true, Priority: 100,
			})
		} else {
			// Was out of scope or unknown → add explicit include (session only).
			_, _ = m.scopeMgr.AddRuleInMemory(scope.Rule{
				Kind: scope.RuleKindHost, Pattern: host,
				MatchMode: scope.MatchModeLiteral, Action: scope.ActionInclude,
				Enabled: true, Priority: 100,
			})
		}
		history.RefreshScopeBadges(m.scopeMgr)
		return true, nil
	case key.Matches(v, key.NewBinding(key.WithKeys("f"))):
		// Toggle scope filter: show all flows vs only in-scope.
		focused := m.ws.FocusedPane()
		if history, ok := focused.View.(*HistoryModel); ok {
			history.scopeFilter = !history.scopeFilter
			history.applyFilter()
		}
		return true, nil

	case key.Matches(v, key.NewBinding(key.WithKeys("p"))):
		// Open preset picker overlay in the focused history pane.
		focused := m.ws.FocusedPane()
		if history, ok := focused.View.(*HistoryModel); ok {
			history.OpenPresetPicker()
		}
		return true, nil

	case key.Matches(v, key.NewBinding(key.WithKeys("C"))):
		// C: clear screen (display only, DB kept).
		m.clearAllHistoryScreens()
		return true, nil

	case key.Matches(v, key.NewBinding(key.WithKeys("D"))):
		// D: wipe persisted history (DB + screen) with confirm.
		m.confirmWipe = true
		return true, nil

	}
	return false, nil
}

func (m *AppModel) openSelectedFlow(open func(*model.Flow) tea.Cmd) (bool, tea.Cmd) {
	focused := m.ws.FocusedPane()
	history, ok := focused.View.(*HistoryModel)
	if !ok {
		return false, nil
	}
	flow := history.SelectedFlow()
	if flow == nil {
		return true, nil
	}
	return true, open(flow)
}

func (m *AppModel) openScopePane() tea.Cmd {
	scopeMgr := m.scopeMgr
	if scopeMgr == nil {
		scopeMgr = scope.NewManager(m.store)
		m.scopeMgr = scopeMgr
		m.repeaterSvc = repeater.NewHTTPService(scopeMgr)
	}
	scopeModel := NewScopeModel(scopeMgr, m.store, m.width, m.height)
	pane := m.ws.SplitHSplit(&scopeView{
		ScopeModel: &scopeModel,
		id:         m.nextViewID("scope"),
	})
	return pane.View.Init()
}

func (m *AppModel) openReconPane() tea.Cmd {
	reconModel := NewReconModel(m.reconMgr, m.scopeMgr, m.width, m.height)
	pane := m.ws.SplitHSplit(&reconView{
		ReconModel: &reconModel,
		id:         m.nextViewID("recon"),
	})
	return pane.View.Init()
}

func (m *AppModel) handleWorkspaceCommand(v workspace.CommandMsg) tea.Cmd {
	// A split command creates a second history pane. This gives the generic
	// workspace manager a concrete view without duplicating mutable model
	// state, while number-key shortcuts open the functional workspaces.
	history := NewHistoryModel(m.store, m.scopeMgr, m.width, m.height)
	history.id = m.nextViewID("history")
	var pane *workspace.Pane
	switch v.Action {
	case workspace.CommandSplitVertical:
		pane = m.ws.SplitVSplit(history)
	case workspace.CommandSplitHorizontal:
		pane = m.ws.SplitHSplit(history)
	default:
		return nil
	}
	return pane.View.Init()
}

func (m *AppModel) nextViewID(prefix string) string {
	m.viewSeq++
	return fmt.Sprintf("%s-%d", prefix, m.viewSeq)
}

// filterReconInScope returns a copy of the summary containing only in-scope hosts and endpoints.
func filterReconInScope(s *recon.ReconSummary, sc *scope.Manager) *recon.ReconSummary {
	if s == nil || sc == nil {
		return s
	}
	filtered := *s
	filtered.Hosts = make([]recon.Host, 0, len(s.Hosts))
	for _, h := range s.Hosts {
		u := &url.URL{Scheme: "https", Host: h.Hostname}
		if sc.Status(u) == model.ScopeInScope {
			filtered.Hosts = append(filtered.Hosts, h)
		}
	}
	filtered.Endpoints = make([]recon.Endpoint, 0, len(s.Endpoints))
	for _, e := range s.Endpoints {
		u, err := url.Parse(e.URL)
		if err == nil && sc.Status(u) == model.ScopeInScope {
			filtered.Endpoints = append(filtered.Endpoints, e)
		}
	}
	return &filtered
}


func (m *AppModel) ensureProjectStore() {
	if m.projectStore == nil {
		ps, _ := project.NewStore("")
		m.projectStore = ps
	}
}

func (m *AppModel) openProjectPicker() {
	if m.projectStore == nil {
		return
	}
	names, _ := m.projectStore.List()
	m.projectList = names
	m.projectCursor = 0
	// Pre-select active project.
	for i, n := range names {
		if n == m.activeProject {
			m.projectCursor = i
			break
		}
	}
	m.projectPicker = true
}

func (m *AppModel) handleProjectPickerKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch v.String() {
	case "esc", "q", "P":
		m.projectPicker = false
		return m, nil
	case "j", "down":
		if m.projectCursor < len(m.projectList)-1 {
			m.projectCursor++
		}
		return m, nil
	case "k", "up":
		if m.projectCursor > 0 {
			m.projectCursor--
		}
		return m, nil
	case "enter", " ":
		if len(m.projectList) == 0 {
			m.projectPicker = false
			return m, nil
		}
		name := m.projectList[m.projectCursor]
		m.projectPicker = false
		rules, err := m.projectStore.Load(name)
		if err != nil {
			return m, nil
		}
		m.scopeMgr.ReplaceRules(rules)
		m.activeProject = name
		for _, p := range m.ws.Layout().Panes() {
			if sv, ok := p.View.(*scopeView); ok {
				sv.ScopeModel.rules = m.scopeMgr.Rules()
				sv.ScopeModel.refreshTable()
			}
		}
		// Re-evaluate scope badges in any history pane.
		for _, p := range m.ws.Layout().Panes() {
			if h, ok := p.View.(*HistoryModel); ok {
				h.RefreshScopeBadges(m.scopeMgr)
			}
		}
		return m, nil
	case "n", "N":
		// Quick-create from picker.
		m.projectPicker = false
		m.creatingProject = true
		m.newProjectInput = textinput.New()
		m.newProjectInput.Prompt = ""
		m.newProjectInput.Placeholder = "new project name"
		m.newProjectInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m *AppModel) handleNewProjectKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(v, key.NewBinding(key.WithKeys("esc"))):
		m.creatingProject = false
		m.newProjectInput.Blur()
		return m, nil
	case key.Matches(v, key.NewBinding(key.WithKeys("enter"))):
		name := strings.TrimSpace(m.newProjectInput.Value())
		m.creatingProject = false
		m.newProjectInput.Blur()
		if name == "" {
			return m, nil
		}
		m.ensureProjectStore()
		rules := m.scopeMgr.Rules()
		_ = m.projectStore.Save(name, rules)
		m.activeProject = name
		// New project = clean slate: clear history display (screen only, DB kept).
		m.clearAllHistoryScreens()
		return m, nil
	default:
		var cmd tea.Cmd
		m.newProjectInput, cmd = m.newProjectInput.Update(v)
		return m, cmd
	}
}

func (m *AppModel) clearAllHistoryScreens() {
	if m.ws == nil {
		return
	}
	for _, p := range m.ws.Layout().Panes() {
		if h, ok := p.View.(*HistoryModel); ok {
			h.ClearScreen()
		}
	}
}

func (m *AppModel) wipeAllHistory() {
	if m.store != nil {
		_ = m.store.ClearFlows(context.Background())
	}
	m.clearAllHistoryScreens()
}

func (m *AppModel) handleConfirmWipeKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch v.String() {
	case "y", "Y", "enter":
		m.confirmWipe = false
		m.wipeAllHistory()
		return m, nil
	case "n", "N", "esc", "q":
		m.confirmWipe = false
		return m, nil
	}
	return m, nil
}

func (m AppModel) renderConfirmWipe() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colRed)).Render("  ⚠  Wipe all HTTP history?")
	b.WriteString("\n" + title + "\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Render("  This deletes ALL flows + analyses from SQLite (persistent)."))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Render("  Use C (clear) for screen-only instead."))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render("  y: confirm wipe   n/esc: cancel"))
	return b.String()
}

func (m AppModel) View() tea.View {
	if !m.ready {
		return tea.View{Content: "Loading...", AltScreen: true}
	}

	var statusBar string
	parts := []string{}
	if m.activeProject != "" {
		parts = append(parts, fmt.Sprintf("project: %s", m.activeProject))
	}
	if m.interceptEnabled {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color(colRed)).Bold(true).Render("● INTERCEPT ON"))
	} else {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Render("intercept: off (I)"))
	}
	if len(parts) > 0 {
		statusBar = " " + strings.Join(parts, " | ")
	}

	// Wipe confirm takes highest priority.
	if m.confirmWipe {
		return tea.View{Content: m.renderConfirmWipe(), AltScreen: true}
	}

	// Project picker / new-project overlays render full-screen.
	if m.projectPicker {
		return tea.View{Content: m.renderProjectPicker(), AltScreen: true}
	}
	if m.creatingProject {
		return tea.View{Content: m.renderNewProjectPrompt(), AltScreen: true}
	}

	if m.ws != nil {
		content := m.ws.View()
		if m.commandMode {
			lines := strings.Split(content, "\n")
			if len(lines) > 0 {
				lines = lines[:len(lines)-1]
			}
			cmdBar := lipgloss.NewStyle().
				Width(m.width).
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("255")).
				Render(":" + m.commandInput.View())
			lines = append(lines, cmdBar)
			content = strings.Join(lines, "\n")
		} else if statusBar != "" {
			lines := strings.Split(content, "\n")
			if len(lines) > 0 {
				last := lines[len(lines)-1]
				if strings.Contains(last, "proxy:") {
					lines[len(lines)-1] = statusBar + " |" + last
				}
			}
			content = strings.Join(lines, "\n")
		}
		// Floating intercept editor overlays everything (centered modal).
		if m.floating != nil {
			// Render floating as centered bordered box over dimmed base.
			fw := int(float64(m.width) * 0.84)
			fh := int(float64(m.height) * 0.86)
			if fw < 40 {
				fw = m.width - 4
			}
			if fh < 10 {
				fh = m.height - 4
			}
			floatingContent := m.floating.View()
			box := lipgloss.NewStyle().
				Width(fw).
				Height(fh).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colAccent)).
				Render(floatingContent)
			overlay := lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, box)
			// Merge base content area (above help/status) with overlay.
			parts := strings.Split(content, "\n")
			helpAndStatus := ""
			if len(parts) >= 2 {
				helpAndStatus = "\n" + parts[len(parts)-2] + "\n" + parts[len(parts)-1]
				parts = parts[:len(parts)-2]
			}
			baseArea := strings.Join(parts, "\n")
			oLines := strings.Split(overlay, "\n")
			bLines := strings.Split(baseArea, "\n")
			maxH := max(len(bLines), len(oLines))
			var merged []string
			for i := 0; i < maxH; i++ {
				var bl, ol string
				if i < len(bLines) {
					bl = bLines[i]
				}
				if i < len(oLines) {
					ol = oLines[i]
				}
				if strings.TrimSpace(ol) == "" {
					merged = append(merged, bl)
				} else {
					merged = append(merged, ol)
				}
			}
			content = strings.Join(merged, "\n") + helpAndStatus
		}
		return tea.View{Content: content, AltScreen: true}
	}

	// Fallback: show history if no workspace.
	if m.history != nil {
		return tea.View{Content: m.history.View(), AltScreen: true}
	}

	// Fallback splash with logo (no workspace/history)
	splash := renderLogoBanner(max(40, m.width)) + "\n\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Align(lipgloss.Center).Width(max(40, m.width)).Render("proxy: "+fmt.Sprintf("%d flows", 0))
	return tea.View{Content: splash, AltScreen: true}
}

func (m AppModel) renderProjectPicker() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colAccent)).Render("  Projects  ")
	sub := lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Render(fmt.Sprintf("(%d saved)", len(m.projectList)))
	b.WriteString("\n" + title + sub + "\n\n")
	if len(m.projectList) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Render("  No projects yet. Press n to create one."))
		b.WriteString("\n")
	} else {
		for i, name := range m.projectList {
			cursor := "  "
			if i == m.projectCursor {
				cursor = lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)).Render("▶ ")
			}
			mark := ""
			if name == m.activeProject {
				mark = lipgloss.NewStyle().Foreground(lipgloss.Color(colGreen)).Render("  ● active")
			}
			style := lipgloss.NewStyle()
			if i == m.projectCursor {
				style = style.Bold(true).Foreground(lipgloss.Color("255"))
			}
			b.WriteString(cursor + style.Render(name) + mark + "\n")
		}
	}
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Render("  j/k: move  enter: open  n: new  esc: cancel"))
	return b.String()
}

func (m AppModel) renderNewProjectPrompt() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colAccent)).Render("  New Project")
	b.WriteString("\n" + title + "\n\n")
	b.WriteString("  Name: " + m.newProjectInput.View() + "\n")
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Render("  enter: save  esc: cancel"))
	return b.String()
}

// NewAppModel creates a new AppModel.
func NewAppModel(s store.Store, p *proxy.Proxy, sc *scope.Manager) *AppModel {
	ws := workspace.NewManager()

	// Create the initial history pane.
	history := NewHistoryModel(s, sc, 80, 24)
	ws.AddPane(history)

	ps, _ := project.NewStore("")

	return &AppModel{
		store:        s,
		proxy:        p,
		scopeMgr:     sc,
		repeaterSvc:  repeater.NewHTTPService(sc),
		ws:           ws,
		history:      history,
		help:         help.New(),
		projectStore: ps,
	}
}
