package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ouroboros/internal/llm"
	"ouroboros/internal/msg"
	"ouroboros/internal/model"
	"ouroboros/internal/proxy"
	"ouroboros/internal/recon"
	"ouroboros/internal/repeater"
	"ouroboros/internal/scope"
	"ouroboros/internal/store"
)

// Mode is the current TUI view mode.
type Mode int

const (
	ModeHistory Mode = iota
	ModeDetail
	ModeRepeater
	ModeLLM
	ModeRecon
	ModeScope
)

// AppModel is the top-level Bubble Tea model for the Ouroboros TUI.
type AppModel struct {
	store                  store.Store
	proxy                  *proxy.Proxy
	repeaterSvc            repeater.Service
	llmAnalyzer            *llm.Analyzer
	reconMgr               *recon.Engine
	reconProgressListening bool
	mode                   Mode
	table                  table.Model
	help                   help.Model
	keymap                 appKeyMap
	quitting               bool
	rows                   []table.Row
	detail                 *DetailModel
	repeater               *RepeaterModel
	llm                    *LLMModel
	recon                  *ReconModel
	scope                  *ScopeModel
	scopeMgr               *scope.Manager
	llmContext             []llm.Message
	lastBulkResult         *llm.BulkAnalysisResult
	width                  int
	height                 int
	ready                  bool
}

type appKeyMap struct {
	quit     key.Binding
	enter    key.Binding
	repeater key.Binding
	llm      key.Binding
	recon    key.Binding
	scope    key.Binding
}

// backToListMsg signals a sub-view to return to the history list.
type backToListMsg struct{}

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
		update, ok := <-ch
		if !ok {
			return nil
		}
		return update
	}
}

func (m *AppModel) Update(mgs tea.Msg) (tea.Model, tea.Cmd) {
	// Handle back-to-list transitions and result messages from any sub-model.
	switch v := mgs.(type) {
	case backToListMsg:
		m.mode = ModeHistory
		m.detail = nil
		m.repeater = nil
		m.llm = nil
		m.recon = nil
		m.scope = nil
		return m, nil
	case repeaterResultMsg:
		if m.repeater != nil {
			setRepeaterResponse(m.repeater, v.resp, v.err)
		}
		return m, nil
	case repeaterScopeBlockMsg:
		if m.repeater != nil {
			m.repeater.scopeBlocked = true
		}
		return m, nil
	case reconResultMsg:
		if m.recon != nil {
			updated, _ := m.recon.Update(v)
			m.recon = &updated
		}
		return m, nil
	case reconAIResultMsg:
		if m.recon != nil {
			updated, _ := m.recon.Update(v)
			m.recon = &updated
		}
		return m, nil
	case recon.ProgressUpdate:
		if m.recon != nil {
			updated, _ := m.recon.Update(v)
			m.recon = &updated
			return m, waitForReconProgress(m.reconMgr.ProgressChan())
		}
		return m, nil
	case reconRunMsg:
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
			// Filter to in-scope assets by default.
			summary := v.summary
			if m.scopeMgr != nil {
				summary = filterReconInScope(summary, m.scopeMgr)
			}
			result, err := m.llmAnalyzer.AnalyzeRecon(context.Background(), summary)
			return reconAIResultMsg{result: result, err: err}
		}
	}

	// Delegate to sub-models based on mode.
	switch m.mode {
	case ModeDetail:
		if m.detail != nil {
			updated, detailCmd := m.detail.Update(mgs)
			m.detail = &updated
			if detailCmd != nil {
				cmdMsg := detailCmd()
				switch v := cmdMsg.(type) {
				case backToListMsg:
					m.mode = ModeHistory
					m.detail = nil
				case msg.ForwardInterceptedFlow:
					if m.proxy != nil {
						m.proxy.HandleInterceptCommand(v)
					}
					m.mode = ModeHistory
					m.detail = nil
				case msg.DropInterceptedFlow:
					if m.proxy != nil {
						m.proxy.HandleInterceptCommandDrop(v)
					}
					m.mode = ModeHistory
					m.detail = nil
				case llmAnalyzeMsg:
					// Switch from detail to LLM view with single-flow analysis.
					l := NewLLMModel(v.flow, m.width, m.height)
					m.mode = ModeLLM
					m.llm = &l
					m.detail = nil
					// Re-dispatch so the LLM handler runs immediately.
					return m, func() tea.Msg { return v }
				}
			}
			return m, nil
		}
	case ModeRepeater:
		if m.repeater != nil {
			updated, repeaterCmd := m.repeater.Update(mgs)
			m.repeater = &updated
			if repeaterCmd != nil {
				cmdMsg := repeaterCmd()
				switch v := cmdMsg.(type) {
				case backToListMsg:
					m.mode = ModeHistory
					m.repeater = nil
				case repeaterSendMsg:
					return m, func() tea.Msg {
						resp, err := m.repeaterSvc.Replay(context.Background(), v.flow, v.edits)
						if errors.Is(err, repeater.ErrOutOfScope) {
							return repeaterScopeBlockMsg{}
						}
						return repeaterResultMsg{resp: resp, err: err}
					}
				}
			}
			return m, nil
		}
	case ModeLLM:
		if m.llm != nil {
			updated, llmCmd := m.llm.Update(mgs)
			m.llm = &updated
			if llmCmd != nil {
				cmdMsg := llmCmd()
				switch v := cmdMsg.(type) {
				case backToListMsg:
					m.mode = ModeHistory
					m.llm = nil
				case llmAnalyzeMsg:
					if v.bulkKind == LLMViewBulk {
						return m, func() tea.Msg {
							if m.llmAnalyzer == nil {
								return llmResultMsg{err: fmt.Errorf("no LLM configured (set OPENAI_API_KEY, NVIDIA_API_KEY, GEMINI_API_KEY, or run Ollama)")}
							}
							flows, _ := m.store.ListFlows(context.Background())
							// Filter to in-scope flows by default.
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
								// Store context for future single-flow analysis.
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
							// Append this Q&A to the running context.
							m.llmContext = append(m.llmContext,
								llm.Message{Role: llm.RoleUser, Content: "Analyzed flow " + v.flow.ID},
								llm.Message{Role: llm.RoleAssistant, Content: result.Summary},
							)
						}
						return llmResultMsg{result: result, err: err}
					}
				}
			}
			return m, nil
		}
	case ModeRecon:
		if m.recon != nil {
			updated, reconCmd := m.recon.Update(mgs)
			m.recon = &updated
			return m, reconCmd
		}
	case ModeScope:
		if m.scope != nil {
			updated, scopeCmd := m.scope.Update(mgs)
			m.scope = &updated
			return m, scopeCmd
		}
	}

	switch v := mgs.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.ready = true
		m.table.SetWidth(v.Width - 2)
		m.table.SetHeight(v.Height - 4)
		m.help.SetWidth(v.Width)
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(v, m.keymap.quit):
			m.quitting = true
			return m, tea.Quit
		case key.Matches(v, m.keymap.enter):
			row := m.table.SelectedRow()
			if row != nil {
				shortID := row[0]
				flows, _ := m.store.ListFlows(context.Background())
				for _, f := range flows {
					if len(f.ID) >= 8 && f.ID[len(f.ID)-8:] == shortID {
						detail := NewDetailModel(f, m.width, m.height)
						m.mode = ModeDetail
						m.detail = &detail
						break
					}
				}
			}
		case key.Matches(v, m.keymap.repeater):
			row := m.table.SelectedRow()
			if row != nil {
				shortID := row[0]
				flows, _ := m.store.ListFlows(context.Background())
				for _, f := range flows {
					if len(f.ID) >= 8 && f.ID[len(f.ID)-8:] == shortID {
						r := NewRepeaterModel(f, m.width, m.height)
						m.mode = ModeRepeater
						m.repeater = &r
						break
					}
				}
			}
		case key.Matches(v, m.keymap.llm):
			// 'a' from history = bulk analysis of all captured traffic.
			l := NewBulkLLMModel(m.width, m.height)
			m.mode = ModeLLM
			m.llm = &l
		case key.Matches(v, m.keymap.recon):
			if m.reconMgr != nil {
				r := NewReconModel(m.reconMgr, m.llmAnalyzer, m.scopeMgr, m.width, m.height)
				m.mode = ModeRecon
				m.recon = &r
			}
		case key.Matches(v, m.keymap.scope):
			if m.scopeMgr != nil {
				sc := NewScopeModel(m.scopeMgr, m.store, m.width, m.height)
				m.mode = ModeScope
				m.scope = &sc
			}
		}

	case msg.FlowCompleted:
		flow := v.Flow
		status := 0
		path := ""
		method := ""
		host := flow.Host
		if flow.Request != nil {
			method = flow.Request.Method
			path = flow.Request.URL
		}
		if flow.Response != nil {
			status = flow.Response.StatusCode
		}
		scopeBadge := "?"
		switch flow.ScopeStatus {
		case model.ScopeInScope:
			scopeBadge = "IN"
		case model.ScopeOutOfScope:
			scopeBadge = "OUT"
		}
		row := table.Row{
			flow.ID[len(flow.ID)-8:],
			method,
			host,
			path,
			strconv.Itoa(status),
			scopeBadge,
			fmt.Sprintf("%dms", flow.Duration.Milliseconds()),
			flow.StartTime.Format("15:04:05"),
		}
		m.rows = append(m.rows, row)
		m.table.SetRows(m.rows)
		m.table.UpdateViewport()
		return m, nil

	case msg.InterceptionRequired:
		flow, err := m.store.GetFlow(nil, v.FlowID)
		if err != nil || flow == nil {
			return m, nil
		}
		detail := NewDetailModel(flow, m.width, m.height)
		m.mode = ModeDetail
		m.detail = &detail
		return m, nil

	case llmResultMsg:
		if m.llm != nil {
			m.llm.result = v.result
			m.llm.bulkResult = v.bulkResult
			m.llm.loading = false
			m.llm.viewport.SetContent(renderLLMResult(v.result, v.bulkResult, v.err))
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(mgs)
	return m, cmd
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
		return tea.NewView("Loading...")
	}

	switch m.mode {
	case ModeDetail:
		if m.detail != nil {
			return m.detail.View()
		}
	case ModeRepeater:
		if m.repeater != nil {
			return m.repeater.View()
		}
	case ModeLLM:
		if m.llm != nil {
			return m.llm.View()
		}
	case ModeRecon:
		if m.recon != nil {
			return m.recon.View()
		}
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).
		Render(" Ouroboros — HTTP History")
	body := m.table.View()
	footer := m.help.ShortHelpView([]key.Binding{
		m.keymap.quit, m.keymap.enter, m.keymap.repeater, m.keymap.llm, m.keymap.recon,
	})

	s := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return tea.View{Content: s, AltScreen: true}
}

// NewAppModel creates a new AppModel.
func NewAppModel(s store.Store, p *proxy.Proxy, sc *scope.Manager) *AppModel {
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Method", Width: 8},
		{Title: "Host", Width: 20},
		{Title: "Path", Width: 30},
		{Title: "Status", Width: 8},
		{Title: "Scope", Width: 6},
		{Title: "Time", Width: 8},
		{Title: "Timestamp", Width: 10},
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
	)

	return &AppModel{
		store:       s,
		proxy:       p,
		scopeMgr:    sc,
		repeaterSvc: repeater.NewHTTPService(sc),
		mode:        ModeHistory,
		table:       t,
		help:        help.New(),
		keymap: appKeyMap{
			quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
			enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "view detail")),
			repeater: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "repeater")),
			llm:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "analyze")),
			recon:    key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "recon")),
			scope:    key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "scope")),
		},
	}
}
