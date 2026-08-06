package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ouroboros/internal/llm"
	"ouroboros/internal/model"
	"ouroboros/internal/msg"
	"ouroboros/internal/project"
	"ouroboros/internal/proxy"
	"ouroboros/internal/recon"
	"ouroboros/internal/repeater"
	"ouroboros/internal/scope"
	"ouroboros/internal/store"
	"ouroboros/internal/workspace"
)

// AppModel is the top-level Bubble Tea model for the Ouroboros TUI.
type AppModel struct {
	store                  store.Store
	proxy                  *proxy.Proxy
	repeaterSvc            repeater.Service
	llmAnalyzer            *llm.Analyzer
	reconMgr               *recon.Engine
	reconProgressListening bool
	scopeMgr               *scope.Manager
	ws                     *workspace.Manager
	history                *HistoryModel
	help                   help.Model
	quitting               bool
	llmContext             []llm.Message
	lastBulkResult         *llm.BulkAnalysisResult
	width                  int
	height                 int
	ready                  bool
	viewSeq                uint64
	commandMode            bool
	commandInput           textinput.Model
	projectStore           *project.Store
	activeProject          string
}

// SetAnalyzer sets the LLM analyzer (called from main after provider config).
func (m *AppModel) SetAnalyzer(a *llm.Analyzer) {
	m.llmAnalyzer = a
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

	case msg.InterceptionRequired:
		flow, err := m.store.GetFlow(context.Background(), v.FlowID)
		if err != nil || flow == nil {
			return m, nil
		}
		// Open detail view in a new pane.
		detail := NewDetailModel(flow, m.width, m.height)
		pane := m.ws.SplitHSplit(&detailView{
			DetailModel: &detail,
			id:          m.nextViewID("detail"),
		})
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

	case reconAIResultMsg:
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

	case reconAIAnalyzeMsg:
		return m, func() tea.Msg {
			if m.llmAnalyzer == nil {
				return reconAIResultMsg{err: fmt.Errorf("no LLM configured")}
			}
			summary := v.summary
			if m.scopeMgr != nil {
				summary = filterReconInScope(summary, m.scopeMgr)
			}
			result, err := m.llmAnalyzer.AnalyzeRecon(context.Background(), summary)
			return reconAIResultMsg{result: result, err: err}
		}

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

	case llmResultMsg:
		if m.ws != nil {
			return m, m.ws.Update(v)
		}
		return m, nil

	case llmAnalyzeMsg:
		if v.bulkKind == LLMViewBulk {
			return m, func() tea.Msg {
				if m.llmAnalyzer == nil {
					return llmResultMsg{err: fmt.Errorf("no LLM configured (set OPENAI_API_KEY, NVIDIA_API_KEY, GEMINI_API_KEY, or run Ollama)")}
				}
				flows, _ := m.store.ListFlows(context.Background())
				if m.scopeMgr != nil {
					var inScope []*model.Flow
					for _, f := range flows {
						if f.ScopeStatus == model.ScopeInScope {
							inScope = append(inScope, f)
						}
					}
					flows = inScope
				}
				result, err := m.llmAnalyzer.AnalyzeBulk(context.Background(), flows)
				if err == nil {
					m.lastBulkResult = result
					m.llmContext = buildBulkContext(result)
				}
				return llmResultMsg{bulkResult: result, err: err}
			}
		}
		return m, func() tea.Msg {
			if m.llmAnalyzer == nil {
				return llmResultMsg{err: fmt.Errorf("no LLM configured (set OPENAI_API_KEY, NVIDIA_API_KEY, GEMINI_API_KEY, or run Ollama)")}
			}
			result, err := m.llmAnalyzer.AnalyzeFlow(context.Background(), v.flow, m.llmContext)
			if err == nil && result != nil {
				m.llmContext = append(m.llmContext,
					llm.Message{Role: llm.RoleUser, Content: "Analyzed flow " + v.flow.ID},
					llm.Message{Role: llm.RoleAssistant, Content: result.Summary},
				)
			}
			return llmResultMsg{result: result, err: err}
		}

	case repeaterSendMsg:
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

// handleGlobalKey processes view-opening shortcuts (0/4/5) that work from
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
	_, err := m.scopeMgr.AddRule(context.Background(), scope.Rule{
		Kind:      scope.RuleKindHost,
		Pattern:   flow.Host,
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
	history := NewHistoryModel(m.store, m.width, m.height)
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

	switch {
	case key.Matches(v, key.NewBinding(key.WithKeys("q"))):
		m.quitting = true
		return true, tea.Quit
	case key.Matches(v, key.NewBinding(key.WithKeys("enter"))):
		return m.openSelectedFlow(func(flow *model.Flow) tea.Cmd {
			detail := NewDetailModel(flow, m.width, m.height)
			pane := m.ws.SplitHSplit(&detailView{
				DetailModel: &detail,
				id:          m.nextViewID("detail"),
			})
			return pane.View.Init()
		})
	case key.Matches(v, key.NewBinding(key.WithKeys("r"))):
		return m.openSelectedFlow(func(flow *model.Flow) tea.Cmd {
			repeater := NewRepeaterModel(flow, m.width, m.height)
			pane := m.ws.SplitHSplit(&repeaterView{
				RepeaterModel: &repeater,
				id:            m.nextViewID("repeater"),
			})
			return pane.View.Init()
		})
	case key.Matches(v, key.NewBinding(key.WithKeys("a"))):
		return m.openSelectedFlow(func(flow *model.Flow) tea.Cmd {
			llmModel := NewLLMModel(flow, m.width, m.height)
			pane := m.ws.SplitHSplit(&llmView{
				LLMModel: &llmModel,
				id:       m.nextViewID("llm"),
			})
			return pane.View.Init()
		})
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
	reconModel := NewReconModel(m.reconMgr, m.llmAnalyzer, m.scopeMgr, m.width, m.height)
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
	history := NewHistoryModel(m.store, m.width, m.height)
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

// buildBulkContext converts a bulk analysis result into LLM conversation
// context for subsequent single-flow analysis.
func buildBulkContext(result *llm.BulkAnalysisResult) []llm.Message {
	if result == nil {
		return nil
	}
	var b strings.Builder
	b.WriteString("Prior bulk traffic analysis:\n")
	b.WriteString("Summary: " + result.Summary + "\n")
	for _, f := range result.Findings {
		b.WriteString(fmt.Sprintf("- flow %s [%s] %s: %s\n", f.FlowID, f.Severity, f.Title, f.Why))
	}
	b.WriteString("\nUse this context when analyzing individual flows.\n")
	return []llm.Message{
		{Role: llm.RoleUser, Content: b.String()},
		{Role: llm.RoleAssistant, Content: "Understood. I will consider this traffic context when analyzing subsequent flows."},
	}
}

func (m AppModel) View() tea.View {
	if !m.ready {
		return tea.View{Content: "Loading...", AltScreen: true}
	}

	var statusBar string
	if m.activeProject != "" {
		statusBar = fmt.Sprintf(" project: %s", m.activeProject)
	}

	if m.ws != nil {
		content := m.ws.View()
		if m.commandMode {
			// Overlay command bar at the very bottom, replacing the status bar.
			lines := strings.Split(content, "\n")
			if len(lines) > 0 {
				lines = lines[:len(lines)-1] // drop status bar
			}
			cmdBar := lipgloss.NewStyle().
				Width(m.width).
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("255")).
				Render(":" + m.commandInput.View())
			lines = append(lines, cmdBar)
			content = strings.Join(lines, "\n")
		} else if statusBar != "" {
			// Inject project name into status bar.
			lines := strings.Split(content, "\n")
			if len(lines) > 0 {
				last := lines[len(lines)-1]
				if strings.Contains(last, "proxy:") {
					lines[len(lines)-1] = statusBar + " |" + last
				}
			}
			content = strings.Join(lines, "\n")
		}
		return tea.View{Content: content, AltScreen: true}
	}

	// Fallback: show history if no workspace.
	if m.history != nil {
		return tea.View{Content: m.history.View(), AltScreen: true}
	}

	return tea.View{Content: "Ouroboros — 0: History  4: Scope  5: Recon  : :command", AltScreen: true}
}

// NewAppModel creates a new AppModel.
func NewAppModel(s store.Store, p *proxy.Proxy, sc *scope.Manager) *AppModel {
	ws := workspace.NewManager()

	// Create the initial history pane.
	history := NewHistoryModel(s, 80, 24)
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
