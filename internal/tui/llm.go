package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ouroboros/internal/llm"
	"ouroboros/internal/model"
)

// LLMViewKind selects what the LLM view shows.
type LLMViewKind int

const (
	LLMViewSingle LLMViewKind = iota
	LLMViewBulk
)

// LLMModel shows the LLM analysis results for a flow or all flows.
type LLMModel struct {
	flow       *model.Flow
	bulkKind   LLMViewKind
	result     *llm.AnalysisResult
	bulkResult *llm.BulkAnalysisResult
	loading    bool
	spinner    spinner.Model
	viewport   viewport.Model
	keymap     llmKeyMap
	help       string
	width      int
	height     int
}

type llmKeyMap struct {
	analyze key.Binding
	back    key.Binding
}

// llmAnalyzeMsg signals the AppModel to analyze the flow.
type llmAnalyzeMsg struct {
	flow     *model.Flow
	bulkKind LLMViewKind
}

// llmResultMsg carries the analysis result back to the TUI.
type llmResultMsg struct {
	result     *llm.AnalysisResult
	bulkResult *llm.BulkAnalysisResult
	err        error
}

func NewLLMModel(flow *model.Flow, width, height int) LLMModel {
	return newLLMModel(flow, LLMViewSingle, width, height)
}

func NewBulkLLMModel(width, height int) LLMModel {
	return newLLMModel(nil, LLMViewBulk, width, height)
}

func newLLMModel(flow *model.Flow, kind LLMViewKind, width, height int) LLMModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	vp := viewport.New(viewport.WithWidth(max(20, width-2)), viewport.WithHeight(max(3, height-4)))
	if kind == LLMViewBulk {
		vp.SetContent("(press a to analyze all captured traffic)")
	} else {
		vp.SetContent("(press a to analyze this flow)")
	}

	help := "a: analyze  q: back"
	if kind == LLMViewBulk {
		help = "a: analyze all  q: back"
	}

	return LLMModel{
		flow:     flow,
		bulkKind: kind,
		spinner:  sp,
		viewport: vp,
		width:    width,
		height:   height,
		keymap: llmKeyMap{
			analyze: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "analyze")),
			back:    key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back")),
		},
		help: help,
	}
}

func (m LLMModel) Init() tea.Cmd {
	return nil
}

func (m LLMModel) Update(mgs tea.Msg) (LLMModel, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.viewport.SetWidth(max(20, v.Width-2))
		m.viewport.SetHeight(max(3, v.Height-4))
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(v, m.keymap.back):
			return m, func() tea.Msg { return backToListMsg{} }
		case key.Matches(v, m.keymap.analyze):
			m.loading = true
			return m, func() tea.Msg { return llmAnalyzeMsg{flow: m.flow, bulkKind: m.bulkKind} }
		}
	case llmResultMsg:
		m.loading = false
		m.result = v.result
		m.bulkResult = v.bulkResult
		m.viewport.SetContent(renderLLMResult(v.result, v.bulkResult, v.err))
		return m, nil
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(mgs)
	m.viewport, cmd = m.viewport.Update(mgs)
	return m, cmd
}

func (m LLMModel) View() tea.View {
	title := " Ouroboros — LLM Analysis"
	if m.bulkKind == LLMViewBulk {
		title = " Ouroboros — LLM Bulk Analysis"
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).
		Width(m.width).Align(lipgloss.Center).
		Render(title)

	body := m.viewport.View()
	if m.loading {
		body = m.spinner.View() + " analyzing..."
	}

	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(m.help)

	s := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return tea.View{Content: s, AltScreen: true}
}

func renderLLMResult(result *llm.AnalysisResult, bulk *llm.BulkAnalysisResult, err error) string {
	if err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("Error: " + err.Error())
	}
	if bulk != nil {
		return renderBulkResult(bulk)
	}
	if result != nil {
		return renderSingleResult(result)
	}
	return "(press a to analyze)"
}

func renderSingleResult(result *llm.AnalysisResult) string {
	var b strings.Builder
	if result.Summary != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("Summary") + "\n")
		b.WriteString(result.Summary + "\n\n")
	}
	if len(result.Findings) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Render("No issues found.") + "\n")
		return b.String()
	}
	for _, f := range result.Findings {
		sevColor := sevToColor(f.Severity)
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(sevColor)).Render(strings.ToUpper(f.Severity)) + " ")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render(f.Title) + "\n")
		b.WriteString(f.Description + "\n")
		if len(f.CVEs) > 0 {
			b.WriteString("CVEs: " + strings.Join(f.CVEs, ", ") + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderBulkResult(result *llm.BulkAnalysisResult) string {
	var b strings.Builder
	if result.Summary != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("Traffic Summary") + "\n")
		b.WriteString(result.Summary + "\n\n")
	}
	if len(result.Findings) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Render("No suspicious requests detected.") + "\n")
		return b.String()
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("Suspicious Requests") + "\n\n")
	for _, f := range result.Findings {
		sevColor := sevToColor(f.Severity)
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(sevColor)).Render(strings.ToUpper(f.Severity)) + " ")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render(f.Title) + " ")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(flow: "+f.FlowID+")") + "\n")
		b.WriteString("Why: " + f.Why + "\n")
		b.WriteString("Fix: " + f.Suggestion + "\n\n")
	}
	return b.String()
}

func sevToColor(sev string) string {
	switch sev {
	case "critical":
		return "196"
	case "high":
		return "208"
	case "medium":
		return "220"
	case "low":
		return "34"
	default:
		return "240"
	}
}
