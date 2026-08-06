package tui

import (
	"context"
	"fmt"
	"strconv"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"sentinel/internal/llm"
	"sentinel/internal/msg"
	"sentinel/internal/proxy"
	"sentinel/internal/repeater"
	"sentinel/internal/store"
)

// Mode is the current TUI view mode.
type Mode int

const (
	ModeHistory Mode = iota
	ModeDetail
	ModeRepeater
	ModeLLM
)

// AppModel is the top-level Bubble Tea model for the Sentinel TUI.
type AppModel struct {
	store       *store.InMemoryFlowStore
	proxy       *proxy.Proxy
	repeaterSvc repeater.Service
	llmAnalyzer *llm.Analyzer
	mode        Mode
	table       table.Model
	help        help.Model
	keymap      appKeyMap
	quitting    bool
	rows        []table.Row
	detail      *DetailModel
	repeater    *RepeaterModel
	llm         *LLMModel
	width       int
	height      int
	ready       bool
}

type appKeyMap struct {
	quit     key.Binding
	enter    key.Binding
	repeater key.Binding
	llm      key.Binding
}

// backToListMsg signals a sub-view to return to the history list.
type backToListMsg struct{}

// SetAnalyzer sets the LLM analyzer (called from main after provider config).
func (m *AppModel) SetAnalyzer(a *llm.Analyzer) {
	m.llmAnalyzer = a
}

func (m *AppModel) Init() tea.Cmd {
	return nil
}

func (m *AppModel) Update(mgs tea.Msg) (tea.Model, tea.Cmd) {
	// Handle back-to-list transitions from any sub-model.
	switch v := mgs.(type) {
	case backToListMsg:
		m.mode = ModeHistory
		m.detail = nil
		m.repeater = nil
		m.llm = nil
		return m, nil
	case repeaterResultMsg:
		if m.repeater != nil {
			setRepeaterResponse(m.repeater, v.resp, v.err)
		}
		return m, nil
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
					return m, func() tea.Msg {
						if m.llmAnalyzer == nil {
							return llmResultMsg{err: fmt.Errorf("no LLM configured (set OPENAI_API_KEY, NVIDIA_API_KEY, or run Ollama)")}
						}
						result, err := m.llmAnalyzer.AnalyzeFlow(context.Background(), v.flow)
						return llmResultMsg{result: result, err: err}
					}
				}
			}
			return m, nil
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
				flows, _ := m.store.List(context.Background())
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
				flows, _ := m.store.List(context.Background())
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
			row := m.table.SelectedRow()
			if row != nil {
				shortID := row[0]
				flows, _ := m.store.List(context.Background())
				for _, f := range flows {
					if len(f.ID) >= 8 && f.ID[len(f.ID)-8:] == shortID {
						l := NewLLMModel(f, m.width, m.height)
						m.mode = ModeLLM
						m.llm = &l
						break
					}
				}
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
		row := table.Row{
			flow.ID[len(flow.ID)-8:],
			method,
			host,
			path,
			strconv.Itoa(status),
			fmt.Sprintf("%dms", flow.Duration.Milliseconds()),
			flow.StartTime.Format("15:04:05"),
		}
		m.rows = append(m.rows, row)
		m.table.SetRows(m.rows)
		m.table.UpdateViewport()
		return m, nil

	case msg.InterceptionRequired:
		flow, err := m.store.Get(nil, v.FlowID)
		if err != nil || flow == nil {
			return m, nil
		}
		detail := NewDetailModel(flow, m.width, m.height)
		m.mode = ModeDetail
		m.detail = &detail
		return m, nil

	case repeaterResultMsg:
		if m.repeater != nil {
			setRepeaterResponse(m.repeater, v.resp, v.err)
		}
		return m, nil

	case llmResultMsg:
		if m.llm != nil {
			m.llm.result = v.result
			m.llm.loading = false
			m.llm.viewport.SetContent(renderLLMResult(v.result, v.err))
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(mgs)
	return m, cmd
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
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).
		Width(m.width).Align(lipgloss.Center).
		Render(" Sentinel — HTTP History")
	body := m.table.View()
	footer := m.help.ShortHelpView([]key.Binding{
		m.keymap.quit, m.keymap.enter, m.keymap.repeater, m.keymap.llm,
	})

	s := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return tea.View{Content: s, AltScreen: true}
}

// NewAppModel creates a new AppModel.
func NewAppModel(s *store.InMemoryFlowStore, p *proxy.Proxy) *AppModel {
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Method", Width: 8},
		{Title: "Host", Width: 20},
		{Title: "Path", Width: 30},
		{Title: "Status", Width: 8},
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
		repeaterSvc: repeater.NewHTTPService(),
		mode:        ModeHistory,
		table:       t,
		help:        help.New(),
		keymap: appKeyMap{
			quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
			enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "view detail")),
			repeater: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "repeater")),
			llm:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "analyze")),
		},
	}
}
