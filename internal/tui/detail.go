package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"sentinel/internal/model"
	"sentinel/internal/msg"
)

// DetailModel shows a single flow's request and response details.
type DetailModel struct {
	flow     *model.Flow
	viewport viewport.Model
	keymap   detailKeyMap
	help     string
}

type detailKeyMap struct {
	forward key.Binding
	drop    key.Binding
	back    key.Binding
}

func NewDetailModel(flow *model.Flow) DetailModel {
	content := renderFlowDetail(flow)
	vp := viewport.New(viewport.WithWidth(100), viewport.WithHeight(20))
	vp.SetContent(content)

	return DetailModel{
		flow:     flow,
		viewport: vp,
		keymap: detailKeyMap{
			forward: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "forward")),
			drop:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "drop")),
			back:    key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back")),
		},
		help: "f: forward  d: drop  q: back",
	}
}

func (m DetailModel) Init() tea.Cmd { return nil }

func (m DetailModel) Update(mgs tea.Msg) (DetailModel, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(v, m.keymap.forward):
			return m, func() tea.Msg { return msg.ForwardInterceptedFlow{FlowID: m.flow.ID} }
		case key.Matches(v, m.keymap.drop):
			return m, func() tea.Msg { return msg.DropInterceptedFlow{FlowID: m.flow.ID} }
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
		Render(" Sentinel — Flow Detail")
	body := m.viewport.View()
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(m.help)

	s := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return tea.NewView(s)
}

// backToListMsg signals the AppModel to return to the history view.
type backToListMsg struct{}

func renderFlowDetail(flow *model.Flow) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Flow ID: ") + flow.ID + "\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Host: ") + flow.Host + "\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Scheme: ") + flow.Scheme + "\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("State: ") + string(flow.State) + "\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Scope: ") + string(flow.ScopeStatus) + "\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Duration: ") + flow.Duration.String() + "\n\n")

	if flow.Request != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("=== Request ===\n"))
		b.WriteString(fmt.Sprintf("%s %s %s\n", flow.Request.Method, flow.Request.URL, flow.Request.HTTPVersion))
		for k, vals := range flow.Request.Headers {
			for _, v := range vals {
				b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
			}
		}
		if len(flow.Request.Body) > 0 {
			b.WriteString("\n" + string(flow.Request.Body) + "\n")
		}
		b.WriteString("\n")
	}

	if flow.Response != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("=== Response ===\n"))
		b.WriteString(fmt.Sprintf("%s %d\n", flow.Response.HTTPVersion, flow.Response.StatusCode))
		for k, vals := range flow.Response.Headers {
			for _, v := range vals {
				b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
			}
		}
		if len(flow.Response.Body) > 0 {
			b.WriteString("\n" + string(flow.Response.Body) + "\n")
		}
		b.WriteString("\n")
	}

	if flow.Error != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("Error: ") + flow.Error + "\n")
	}

	return b.String()
}
