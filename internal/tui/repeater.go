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
	flow      *model.Flow
	methodIn  textinput.Model
	urlIn     textinput.Model
	headersIn textarea.Model
	bodyIn    textarea.Model
	respView  viewport.Model
	keymap    repeaterKeyMap
	help      string
	resp      *model.Message
	width     int
	height    int
	focusIdx  int // 0=method, 1=url, 2=headers, 3=body
}

type repeaterKeyMap struct {
	send key.Binding
	back key.Binding
	next key.Binding
	prev key.Binding
}

func NewRepeaterModel(flow *model.Flow, width, height int) RepeaterModel {
	methodIn := textinput.New()
	methodIn.Placeholder = "GET"
	methodIn.Focus()

	urlIn := textinput.New()
	urlIn.Placeholder = "https://example.com/api"

	headersIn := textarea.New()
	headersIn.Placeholder = "Content-Type: application/json"

	bodyIn := textarea.New()
	bodyIn.Placeholder = `{"key": "value"}`

	respView := viewport.New(viewport.WithWidth(width-2), viewport.WithHeight(height/3))

	m := RepeaterModel{
		flow:      flow,
		methodIn:  methodIn,
		urlIn:     urlIn,
		headersIn: headersIn,
		bodyIn:    bodyIn,
		respView:  respView,
		width:     width,
		height:    height,
		keymap: repeaterKeyMap{
			send: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
			back: key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back")),
			next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
			prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
		},
		help: "tab: next  shift+tab: prev  enter: send  q: back",
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
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.respView.SetWidth(v.Width - 2)
		m.respView.SetHeight(v.Height / 3)
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(v, m.keymap.back):
			return m, func() tea.Msg { return backToListMsg{} }
		case key.Matches(v, m.keymap.next):
			m.focusIdx = (m.focusIdx + 1) % 4
			m.updateFocus()
			return m, nil
		case key.Matches(v, m.keymap.prev):
			m.focusIdx = (m.focusIdx + 3) % 4
			m.updateFocus()
			return m, nil
		case key.Matches(v, m.keymap.send):
			// Only send when focused on body (last field) and enter pressed.
			if m.focusIdx == 3 {
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
	}

	var cmd tea.Cmd
	m.methodIn, cmd = m.methodIn.Update(mgs)
	m.urlIn, cmd = m.urlIn.Update(mgs)
	m.headersIn, cmd = m.headersIn.Update(mgs)
	m.bodyIn, cmd = m.bodyIn.Update(mgs)
	m.respView, cmd = m.respView.Update(mgs)
	return m, cmd
}

func (m *RepeaterModel) updateFocus() {
	m.methodIn.Blur()
	m.urlIn.Blur()
	m.headersIn.Blur()
	m.bodyIn.Blur()
	switch m.focusIdx {
	case 0:
		m.methodIn.Focus()
	case 1:
		m.urlIn.Focus()
	case 2:
		m.headersIn.Focus()
	case 3:
		m.bodyIn.Focus()
	}
}

func (m RepeaterModel) View() tea.View {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).
		Width(m.width).Align(lipgloss.Center).
		Render(" Sentinel — Repeater")

	// Request section — use half the available height.
	reqHeight := m.height/2 - 4

	methodLine := lipgloss.NewStyle().Width(10).Render("Method:") + m.methodIn.View()
	urlLine := lipgloss.NewStyle().Width(10).Render("URL:") + m.urlIn.View()

	m.headersIn.SetWidth(m.width - 14)
	m.headersIn.SetHeight(reqHeight / 4)
	headersLine := lipgloss.NewStyle().Width(10).Render("Headers:") + "\n" + m.headersIn.View()

	m.bodyIn.SetWidth(m.width - 14)
	m.bodyIn.SetHeight(reqHeight / 2)
	bodyLine := lipgloss.NewStyle().Width(10).Render("Body:") + "\n" + m.bodyIn.View()

	// Response section.
	respHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("Response")
	respContent := m.respView.View()
	if m.resp == nil {
		respContent = "(no response yet — tab to body field, then press enter to send)"
	}

	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(m.help)

	s := lipgloss.JoinVertical(lipgloss.Left,
		header,
		methodLine,
		urlLine,
		headersLine,
		bodyLine,
		respHeader,
		respContent,
		footer,
	)
	return tea.View{Content: s, AltScreen: true}
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
