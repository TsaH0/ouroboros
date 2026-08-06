package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"sentinel/internal/llm"
	"sentinel/internal/model"
)

// LLMModel shows the LLM analysis results for a flow.
type LLMModel struct {
	flow     *model.Flow
	result   *llm.AnalysisResult
	loading  bool
	spinner  spinner.Model
	viewport viewport.Model
	keymap   llmKeyMap
	help     string
}

type llmKeyMap struct {
	analyze key.Binding
	back    key.Binding
}

// llmAnalyzeMsg signals the AppModel to analyze the flow.
type llmAnalyzeMsg struct {
	flow *model.Flow
}

// llmResultMsg carries the analysis result back to the TUI.
type llmResultMsg struct {
	result *llm.AnalysisResult
	err    error
}

func NewLLMModel(flow *model.Flow) LLMModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	vp := viewport.New(viewport.WithWidth(100), viewport.WithHeight(20))
	vp.SetContent("(press a to analyze)")

	return LLMModel{
		flow:     flow,
		spinner:  sp,
		viewport: vp,
		keymap: llmKeyMap{
			analyze: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "analyze")),
			back:    key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back")),
		},
		help: "a: analyze  q: back",
	}
}

func (m LLMModel) Init() tea.Cmd {
	return nil
}

func (m LLMModel) Update(mgs tea.Msg) (LLMModel, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(v, m.keymap.back):
			return m, func() tea.Msg { return backToListMsg{} }
		case key.Matches(v, m.keymap.analyze):
			m.loading = true
			return m, func() tea.Msg { return llmAnalyzeMsg{flow: m.flow} }
		}
	case llmResultMsg:
		m.loading = false
		m.result = v.result
		m.viewport.SetContent(renderLLMResult(v.result, v.err))
		return m, nil
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(mgs)
	m.viewport, cmd = m.viewport.Update(mgs)
	return m, cmd
}

func (m LLMModel) View() tea.View {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(" Sentinel — LLM Analysis")

	body := m.viewport.View()
	if m.loading {
		body = m.spinner.View() + " analyzing..."
	}

	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(m.help)

	s := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return tea.NewView(s)
}

func renderLLMResult(result *llm.AnalysisResult, err error) string {
	if err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("Error: " + err.Error())
	}
	if result == nil {
		return "(press a to analyze)"
	}
	if len(result.Findings) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Render("No issues found.")
	}

	var b strings.Builder
	for _, f := range result.Findings {
		sevColor := "240"
		switch f.Severity {
		case "critical":
			sevColor = "196"
		case "high":
			sevColor = "208"
		case "medium":
			sevColor = "220"
		case "low":
			sevColor = "34"
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(sevColor)).Render(strings.ToUpper(f.Severity)) + " " + lipgloss.NewStyle().Bold(true).Render(f.Title) + "\n")
		b.WriteString(f.Description + "\n")
		if len(f.CVEs) > 0 {
			b.WriteString("CVEs: " + strings.Join(f.CVEs, ", ") + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}
