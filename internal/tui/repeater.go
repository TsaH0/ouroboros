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

	"ouroboros/internal/model"
	"ouroboros/internal/repeater"
)

// RepeaterModel shows an editable request form and the replay response.
type RepeaterModel struct {
	flow         *model.Flow
	methodIn     textinput.Model
	urlIn        textinput.Model
	headersIn    textarea.Model
	bodyIn       textarea.Model
	respView     viewport.Model
	keymap       repeaterKeyMap
	resp         *model.Message
	respErr      error
	width        int
	height       int
	focusIdx     int  // 0=method, 1=url, 2=headers, 3=body, 4=response
	editing      bool // vim-style insert mode for the selected request field
	scopeBlocked bool // true when out-of-scope and awaiting confirmation
}

type repeaterKeyMap struct {
	send     key.Binding
	sendEdit key.Binding
	back     key.Binding
	next     key.Binding
	prev     key.Binding
	edit     key.Binding
	done     key.Binding
}

func NewRepeaterModel(flow *model.Flow, width, height int) RepeaterModel {
	methodIn := textinput.New()
	methodIn.Placeholder = "GET"

	urlIn := textinput.New()
	urlIn.Placeholder = "https://example.com/api"

	headersIn := textarea.New()
	headersIn.Placeholder = "Content-Type: application/json"

	bodyIn := textarea.New()
	bodyIn.Placeholder = `{"key": "value"}`

	respView := viewport.New(viewport.WithWidth(max(20, width-2)), viewport.WithHeight(max(3, height/4)))

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
			send:     key.NewBinding(key.WithKeys("enter", "s", "f5", "ctrl+j"), key.WithHelp("enter/s/f5", "send")),
			sendEdit: key.NewBinding(key.WithKeys("s", "f5", "ctrl+j"), key.WithHelp("s/f5", "send")),
			back:     key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back")),
			next:     key.NewBinding(key.WithKeys("j", "down", "tab"), key.WithHelp("j/tab", "next")),
			prev:     key.NewBinding(key.WithKeys("k", "up", "shift+tab"), key.WithHelp("k", "prev")),
			edit:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "edit")),
			done:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "normal")),
		},
	}

	if flow.Request != nil {
		m.methodIn.SetValue(flow.Request.Method)
		m.urlIn.SetValue(flow.Request.URL)
		m.headersIn.SetValue(renderHeaders(flow.Request.Headers))
		m.bodyIn.SetValue(string(flow.Request.Body))
	}
	m.updateFocus()

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
		m.respView.SetWidth(max(20, v.Width-2))
		m.respView.SetHeight(max(3, v.Height/4))
		return m, nil
	case tea.KeyPressMsg:
		if m.scopeBlocked {
			switch {
			case key.Matches(v, key.NewBinding(key.WithKeys("y", "Y"))):
				m.scopeBlocked = false
				return m, m.sendCmd()
			case key.Matches(v, key.NewBinding(key.WithKeys("n", "N", "esc"))):
				m.scopeBlocked = false
				return m, nil
			}
			return m, nil
		}
		if m.editing {
			switch {
			case key.Matches(v, m.keymap.done):
				m.editing = false
				m.updateFocus()
				return m, nil
			case key.Matches(v, m.keymap.sendEdit):
				return m, m.sendCmd()
			case key.Matches(v, m.keymap.next):
				m.focusIdx = (m.focusIdx + 1) % 4
				m.updateFocus()
				return m, nil
			case key.Matches(v, m.keymap.prev):
				m.focusIdx = (m.focusIdx + 3) % 4
				m.updateFocus()
				return m, nil
			}
		} else {
			switch {
			case key.Matches(v, m.keymap.back):
				return m, func() tea.Msg { return backToListMsg{} }
			case key.Matches(v, m.keymap.next):
				m.focusIdx = (m.focusIdx + 1) % 5
				m.updateFocus()
				return m, nil
			case key.Matches(v, m.keymap.prev):
				m.focusIdx = (m.focusIdx + 4) % 5
				m.updateFocus()
				return m, nil
			case key.Matches(v, m.keymap.edit):
				if m.focusIdx < 4 {
					m.editing = true
					m.updateFocus()
				}
				return m, nil
			case key.Matches(v, m.keymap.send):
				return m, m.sendCmd()
			}
		}
	}

	if m.editing {
		return m.updateFocusedInput(mgs)
	}
	if m.focusIdx == 4 {
		var cmd tea.Cmd
		m.respView, cmd = m.respView.Update(mgs)
		return m, cmd
	}
	return m, nil
}

func (m RepeaterModel) sendCmd() tea.Cmd {
	return func() tea.Msg {
		return repeaterSendMsg{
			flow: m.flow,
			edits: repeater.Edits{
				Method:  strings.TrimSpace(m.methodIn.Value()),
				URL:     strings.TrimSpace(m.urlIn.Value()),
				Headers: parseHeaders(m.headersIn.Value()),
				Body:    []byte(m.bodyIn.Value()),
			},
		}
	}
}

func (m RepeaterModel) updateFocusedInput(mgs tea.Msg) (RepeaterModel, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focusIdx {
	case 0:
		m.methodIn, cmd = m.methodIn.Update(mgs)
	case 1:
		m.urlIn, cmd = m.urlIn.Update(mgs)
	case 2:
		m.headersIn, cmd = m.headersIn.Update(mgs)
	case 3:
		m.bodyIn, cmd = m.bodyIn.Update(mgs)
	}
	return m, cmd
}

func (m *RepeaterModel) updateFocus() {
	m.methodIn.Blur()
	m.urlIn.Blur()
	m.headersIn.Blur()
	m.bodyIn.Blur()
	if !m.editing {
		return
	}
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
	width := max(40, m.width)
	height := max(16, m.height)
	contentWidth := max(20, width-4)

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Width(width).Align(lipgloss.Center)
	activeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	labelStyle := lipgloss.NewStyle().Width(10)

	modeText := "NORMAL"
	if m.editing {
		modeText = "INSERT"
	}
	header := headerStyle.Render(" Ouroboros — Repeater [" + modeText + "]")

	tallArea := max(6, height-9)
	headersHeight := clamp(tallArea/3, 5, max(5, tallArea-5))
	bodyHeight := max(5, tallArea-headersHeight)
	respHeight := max(3, height-(headersHeight+bodyHeight+8))

	m.methodIn.SetWidth(max(8, contentWidth-10))
	m.urlIn.SetWidth(max(12, contentWidth-10))
	m.headersIn.SetWidth(contentWidth)
	m.headersIn.SetHeight(headersHeight)
	m.bodyIn.SetWidth(contentWidth)
	m.bodyIn.SetHeight(bodyHeight)
	m.respView.SetWidth(contentWidth)
	m.respView.SetHeight(respHeight)

	methodLine := m.renderFieldLabel(0, labelStyle, activeStyle, "Method:") + m.methodIn.View()
	urlLine := m.renderFieldLabel(1, labelStyle, activeStyle, "URL:") + m.urlIn.View()
	headersLine := m.renderFieldLabel(2, labelStyle, activeStyle, "Headers:") + "\n" + m.headersIn.View()
	bodyLine := m.renderFieldLabel(3, labelStyle, activeStyle, "Body:") + "\n" + m.bodyIn.View()

	respLabel := "Response"
	if m.focusIdx == 4 {
		respLabel = activeStyle.Render("▶ Response")
	} else {
		respLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("Response")
	}
	respContent := m.respView.View()
	if m.scopeBlocked {
		respContent = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render("OUT OF SCOPE — press Y to send anyway, N to cancel")
	} else if m.resp == nil && m.respErr == nil {
		respContent = "(no response yet — press enter, s, F5, or ctrl+j from normal mode to send)"
	}

	help := "NORMAL: j/k/tab move  i edit  enter/s/f5 send  q back | INSERT: esc normal  tab next  s/f5 send"
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).MaxWidth(width).Render(help)

	s := lipgloss.JoinVertical(lipgloss.Left,
		header,
		methodLine,
		urlLine,
		headersLine,
		bodyLine,
		respLabel,
		respContent,
		footer,
	)
	return tea.View{Content: s, AltScreen: true}
}

func (m RepeaterModel) renderFieldLabel(idx int, base, active lipgloss.Style, label string) string {
	if m.focusIdx == idx {
		return active.Render("▶ " + strings.TrimSuffix(label, ":") + ":")
	}
	return base.Render(label)
}

// repeaterSendMsg signals the AppModel to send the replay request.
type repeaterSendMsg struct {
	flow  *model.Flow
	edits repeater.Edits
}

// repeaterScopeBlockMsg is sent when the repeater target is out of scope.
type repeaterScopeBlockMsg struct{}

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

func setRepeaterResponse(m *RepeaterModel, resp *model.Message, err error) {
	m.resp = resp
	m.respErr = err
	var b bytes.Buffer
	if err != nil {
		fmt.Fprintf(&b, "Replay error: %v\n", err)
		m.respView.SetContent(b.String())
		return
	}
	if resp == nil {
		m.respView.SetContent("Replay returned no response.")
		return
	}
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
