package tui

import (
	"bytes"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"sentinel/internal/model"
	"sentinel/internal/repeater"
)

// RepeaterModel shows an editable request form and the replay response.
type RepeaterModel struct {
	flow     *model.Flow
	methodIn textinput.Model
	urlIn    textinput.Model
	headersIn textarea.Model
	bodyIn   textarea.Model
	respView viewport.Model
	keymap   repeaterKeyMap
	help     string
	resp     *model.Message
}

type repeaterKeyMap struct {
	send key.Binding
	back key.Binding
}

func NewRepeaterModel(flow *model.Flow) RepeaterModel {
	methodIn := textinput.New()
	methodIn.Placeholder = "GET"
	methodIn.SetWidth(10)

	urlIn := textinput.New()
	urlIn.Placeholder = "https://example.com/api"
	urlIn.SetWidth(80)

	headersIn := textarea.New()
	headersIn.Placeholder = "Content-Type: application/json"
	headersIn.SetWidth(100)
	headersIn.SetHeight(5)

	bodyIn := textarea.New()
	bodyIn.Placeholder = "{\"key\": \"value\"}"
	bodyIn.SetWidth(100)
	bodyIn.SetHeight(10)

	respView := viewport.New(viewport.WithWidth(100), viewport.WithHeight(15))

	m := RepeaterModel{
		flow:      flow,
		methodIn:  methodIn,
		urlIn:     urlIn,
		headersIn: headersIn,
		bodyIn:    bodyIn,
		respView:  respView,
		keymap: repeaterKeyMap{
			send: key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "send")),
			back: key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back")),
		},
		help: "ctrl+s: send  q: back",
	}

	// Pre-fill from flow.
	if flow.Request != nil {
		m.methodIn.SetValue(flow.Request.Method)
		m.urlIn.SetValue(flow.Request.URL)
		m.headersIn.SetValue(renderHeaders(flow.Request.Headers))
		m.bodyIn.SetValue(string(flow.Request.Body))
	}

	return m
}

func (m RepeaterModel) Init() tea.Cmd {
	return nil
}

func (m RepeaterModel) Update(mgs tea.Msg) (RepeaterModel, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(v, m.keymap.back):
			return m, func() tea.Msg { return backToListMsg{} }
		case key.Matches(v, m.keymap.send):
			return m, func() tea.Msg {
				return repeaterSendMsg{
					flow: m.flow,
					edits: repeater.Edits{
						Method:  m.methodIn.Value(),
						URL:     m.urlIn.Value(),
						Headers: parseHeaders(m.headersIn.Value()),
						Body:    []byte(m.bodyIn.Value()),
					},
				}
			}
		}
	}

	var cmd tea.Cmd
	m.methodIn, cmd = m.methodIn.Update(mgs)
	m.urlIn, cmd = m.urlIn.Update(mgs)
	m.headersIn, cmd = m.headersIn.Update(mgs)
	m.bodyIn, cmd = m.bodyIn.Update(mgs)
	m.respView, cmd = m.respView.Update(mgs)
	return m, cmd
}

func (m RepeaterModel) View() tea.View {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(" Sentinel — Repeater")

	requestSection := lipgloss.NewStyle().Bold(true).Render("=== Request ===")
	methodLine := lipgloss.NewStyle().Width(12).Render("Method:") + m.methodIn.View()
	urlLine := lipgloss.NewStyle().Width(12).Render("URL:") + m.urlIn.View()
	headersLine := lipgloss.NewStyle().Width(12).Render("Headers:") + "\n" + m.headersIn.View()
	bodyLine := lipgloss.NewStyle().Width(12).Render("Body:") + "\n" + m.bodyIn.View()

	responseSection := lipgloss.NewStyle().Bold(true).Render("=== Response ===")
	respContent := m.respView.View()
	if m.resp == nil {
		respContent = "(no response yet)"
	}

	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(m.help)

	s := lipgloss.JoinVertical(lipgloss.Left,
		header,
		requestSection,
		methodLine,
		urlLine,
		headersLine,
		bodyLine,
		responseSection,
		respContent,
		footer,
	)
	return tea.NewView(s)
}

// repeaterSendMsg signals the AppModel to send the replay request.
type repeaterSendMsg struct {
	flow  *model.Flow
	edits repeater.Edits
}

// repeaterResultMsg carries the replay response back to the TUI.
type repeaterResultMsg struct {
	resp *model.Message
	err  error
}

func renderHeaders(h map[string][]string) string {
	var b strings.Builder
	for k, vals := range h {
		for _, v := range vals {
			b.WriteString(k + ": " + v + "\n")
		}
	}
	return b.String()
}

func parseHeaders(s string) map[string][]string {
	h := make(map[string][]string)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		h[k] = append(h[k], v)
	}
	return h
}

func setRepeaterResponse(m *RepeaterModel, resp *model.Message) {
	m.resp = resp
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s %d\n", resp.HTTPVersion, resp.StatusCode)
	for k, vals := range resp.Headers {
		for _, v := range vals {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	if len(resp.Body) > 0 {
		fmt.Fprintf(&b, "\n%s\n", string(resp.Body))
	}
	m.respView.SetContent(b.String())
}
