package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ouroboros/internal/model"
	"ouroboros/internal/msg"
)

// DetailModel shows a single flow's request and response details.
type DetailModel struct {
	flow     *model.Flow
	viewport viewport.Model
	keymap   detailKeyMap
	width    int
	height   int
}

type detailKeyMap struct {
	forward key.Binding
	drop    key.Binding
	analyze key.Binding
	back    key.Binding
}

func NewDetailModel(flow *model.Flow, width, height int) DetailModel {
	vp := viewport.New(viewport.WithWidth(width-2), viewport.WithHeight(height-4))
	vp.SetContent(renderFlowDetail(flow))

	return DetailModel{
		flow:     flow,
		viewport: vp,
		width:    width,
		height:   height,
		keymap: detailKeyMap{
			forward: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "forward")),
			drop:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "drop")),
			analyze: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "analyze")),
			back:    key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back")),
		},
	}
}

func (m DetailModel) Init() tea.Cmd {
	return nil
}

func (m DetailModel) Update(mgs tea.Msg) (DetailModel, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.viewport.SetWidth(v.Width - 2)
		m.viewport.SetHeight(v.Height - 4)
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(v, m.keymap.forward):
			return m, func() tea.Msg { return msg.ForwardInterceptedFlow{FlowID: m.flow.ID} }
		case key.Matches(v, m.keymap.drop):
			return m, func() tea.Msg { return msg.DropInterceptedFlow{FlowID: m.flow.ID} }
		case key.Matches(v, m.keymap.analyze):
			return m, func() tea.Msg { return llmAnalyzeMsg{flow: m.flow, bulkKind: LLMViewSingle} }
		case key.Matches(v, m.keymap.back):
			return m, func() tea.Msg { return backToListMsg{} }
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(mgs)
	return m, cmd
}

func (m DetailModel) View() tea.View {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).
		Width(m.width).Align(lipgloss.Center).
		Render(" Ouroboros — Flow Detail")

	helpLine := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
		Render("f: forward  d: drop  a: analyze  q: back")

	body := m.viewport.View()

	s := lipgloss.JoinVertical(lipgloss.Left, header, body, helpLine)
	return tea.View{Content: s, AltScreen: true}
}

func renderFlowDetail(flow *model.Flow) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("Request") + "\n")
	if flow.Request != nil {
		b.WriteString(fmt.Sprintf("Method: %s\n", flow.Request.Method))
		b.WriteString(fmt.Sprintf("URL: %s\n", flow.Request.URL))
		b.WriteString(fmt.Sprintf("Version: %s\n", flow.Request.HTTPVersion))
		for k, vals := range flow.Request.Headers {
			for _, v := range vals {
				b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
			}
		}
		if len(flow.Request.Body) > 0 {
			b.WriteString(fmt.Sprintf("\n%s\n", string(flow.Request.Body)))
		}
	}

	b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("Response") + "\n")
	if flow.Response != nil {
		b.WriteString(fmt.Sprintf("Status: %d\n", flow.Response.StatusCode))
		b.WriteString(fmt.Sprintf("Version: %s\n", flow.Response.HTTPVersion))
		for k, vals := range flow.Response.Headers {
			for _, v := range vals {
				b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
			}
		}
		if len(flow.Response.Body) > 0 {
			b.WriteString(fmt.Sprintf("\n%s\n", string(flow.Response.Body)))
		}
	}

	if flow.Error != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("Error: ") + flow.Error + "\n")
	}

	return b.String()
}
