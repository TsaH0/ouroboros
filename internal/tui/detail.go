package tui

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
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
	editing  bool
	methodIn textinput.Model
	urlIn    textinput.Model
	headersIn textarea.Model
	bodyIn   textarea.Model
	focusIdx int // 0=method,1=url,2=headers,3=body
}

type detailKeyMap struct {
	forward  key.Binding
	drop     key.Binding
	back     key.Binding
	edit     key.Binding
	done     key.Binding
	next     key.Binding
	prev     key.Binding
	repeater key.Binding
}

// detailOpenRepeaterMsg asks AppModel to open a repeater for the flow.
type detailOpenRepeaterMsg struct {
	flow *model.Flow
}

func NewDetailModel(flow *model.Flow, width, height int) DetailModel {
	vp := viewport.New(viewport.WithWidth(width-2), viewport.WithHeight(height-4))
	vp.SetContent(renderFlowDetail(flow))

	methodIn := textinput.New()
	methodIn.Placeholder = "GET"
	urlIn := textinput.New()
	urlIn.Placeholder = "https://example.com/api"
	headersIn := textarea.New()
	headersIn.Placeholder = "Content-Type: application/json"
	bodyIn := textarea.New()
	bodyIn.Placeholder = `{"key": "value"}`
	if flow.Request != nil {
		methodIn.SetValue(flow.Request.Method)
		urlIn.SetValue(flow.Request.URL)
		headersIn.SetValue(renderHeadersForEdit(flow.Request.Headers))
		bodyIn.SetValue(string(flow.Request.Body))
	}

	return DetailModel{
		flow:      flow,
		viewport:  vp,
		methodIn:  methodIn,
		urlIn:     urlIn,
		headersIn: headersIn,
		bodyIn:    bodyIn,
		width:     width,
		height:    height,
		keymap: detailKeyMap{
			forward:  key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "forward")),
			drop:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "drop")),
			back:     key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back")),
			edit:     key.NewBinding(key.WithKeys("e", "i"), key.WithHelp("e", "edit")),
			done:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "done")),
			next:     key.NewBinding(key.WithKeys("tab", "j", "down"), key.WithHelp("tab", "next field")),
			prev:     key.NewBinding(key.WithKeys("shift+tab", "k", "up"), key.WithHelp("shift+tab", "prev")),
			repeater: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "repeater")),
		},
	}
}

func (m *DetailModel) isIntercepted() bool {
	return m.flow != nil && m.flow.State == model.FlowIntercepted
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
		if m.editing {
			m.headersIn.SetWidth(max(20, v.Width-4))
			m.bodyIn.SetWidth(max(20, v.Width-4))
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.editing {
			switch {
			case key.Matches(v, m.keymap.done):
				m.editing = false
				m.methodIn.Blur()
				m.urlIn.Blur()
				m.headersIn.Blur()
				m.bodyIn.Blur()
				return m, nil
			case key.Matches(v, m.keymap.next):
				m.focusIdx = (m.focusIdx + 1) % 4
				m.updateEditFocus()
				return m, nil
			case key.Matches(v, m.keymap.prev):
				m.focusIdx = (m.focusIdx + 3) % 4
				m.updateEditFocus()
				return m, nil
			case key.Matches(v, m.keymap.forward):
				// Forward with current edits even while in insert mode
				return m, m.forwardCmd()
			case key.Matches(v, m.keymap.drop):
				return m, func() tea.Msg { return msg.DropInterceptedFlow{FlowID: m.flow.ID} }
			}
			return m.updateEditInputs(mgs)
		}
		switch {
		case key.Matches(v, m.keymap.forward):
			return m, m.forwardCmd()
		case key.Matches(v, m.keymap.drop):
			return m, func() tea.Msg { return msg.DropInterceptedFlow{FlowID: m.flow.ID} }
		case key.Matches(v, m.keymap.edit):
			if m.isIntercepted() {
				m.editing = true
				m.focusIdx = 0
				m.updateEditFocus()
				return m, nil
			}
		case key.Matches(v, m.keymap.repeater):
			return m, func() tea.Msg { return detailOpenRepeaterMsg{flow: m.flow} }
		case key.Matches(v, m.keymap.back):
			return m, func() tea.Msg { return backToListMsg{} }
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(mgs)
	return m, cmd
}

func (m DetailModel) forwardCmd() tea.Cmd {
	return func() tea.Msg {
		if m.isIntercepted() {
			return msg.ForwardInterceptedFlow{
				FlowID: m.flow.ID,
				Edited: &msg.EditedRequest{
					Method:  strings.TrimSpace(m.methodIn.Value()),
					URL:     strings.TrimSpace(m.urlIn.Value()),
					Headers: parseHeadersForDetail(m.headersIn.Value()),
					Body:    []byte(m.bodyIn.Value()),
				},
			}
		}
		return msg.ForwardInterceptedFlow{FlowID: m.flow.ID}
	}
}

func (m *DetailModel) updateEditFocus() {
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

func (m DetailModel) updateEditInputs(mgs tea.Msg) (DetailModel, tea.Cmd) {
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

func (m DetailModel) View() tea.View {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).
		Width(m.width).Align(lipgloss.Center).
		Render(" Ouroboros — Flow Detail")

	if m.editing && m.isIntercepted() {
		header = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).
			Width(m.width).Align(lipgloss.Center).
			Render(" Ouroboros — Intercept Edit [INSERT] ")
		labelStyle := lipgloss.NewStyle().Width(10)
		activeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
		contentWidth := max(20, m.width-4)
		m.methodIn.SetWidth(max(8, contentWidth-10))
		m.urlIn.SetWidth(max(12, contentWidth-10))
		m.headersIn.SetWidth(contentWidth)
		m.headersIn.SetHeight(max(5, (m.height-12)/2))
		m.bodyIn.SetWidth(contentWidth)
		m.bodyIn.SetHeight(max(5, (m.height-12)/2))
		methodLine := m.renderFieldLabel(0, labelStyle, activeStyle, "Method:") + m.methodIn.View()
		urlLine := m.renderFieldLabel(1, labelStyle, activeStyle, "URL:") + m.urlIn.View()
		headersLine := m.renderFieldLabel(2, labelStyle, activeStyle, "Headers:") + "\n" + m.headersIn.View()
		bodyLine := m.renderFieldLabel(3, labelStyle, activeStyle, "Body:") + "\n" + m.bodyIn.View()
		helpLine := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("tab/j/k: field  e: edit  f: forward edited  d: drop  esc: done  q: back")
		s := lipgloss.JoinVertical(lipgloss.Left, header, methodLine, urlLine, headersLine, bodyLine, helpLine)
		return tea.View{Content: s, AltScreen: true}
	}

	helpLine := "f: forward  d: drop  q: back"
	if m.isIntercepted() {
		helpLine = "f: forward  d: drop  e: edit  r: repeater  q: back  (edited on e)"
	}
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(helpLine)
	body := m.viewport.View()
	s := lipgloss.JoinVertical(lipgloss.Left, header, body, help)
	return tea.View{Content: s, AltScreen: true}
}

func (m DetailModel) renderFieldLabel(idx int, base, active lipgloss.Style, label string) string {
	if m.focusIdx == idx {
		return active.Render("▶ " + strings.TrimSuffix(label, ":") + ":")
	}
	return base.Render(label)
}

func maxDetail(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ = maxDetail // avoid unused if max defined elsewhere

func renderHeadersForEdit(h map[string][]string) string {
	var b strings.Builder
	for k, vals := range h {
		for _, v := range vals {
			b.WriteString(k + ": " + v + "\n")
		}
	}
	return b.String()
}

func parseHeadersForDetail(s string) map[string][]string {
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
			b.WriteString("\n" + renderBody(flow.Request.Body, flow.Request.Headers, "Request Body") + "\n")
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
			b.WriteString("\n" + renderBody(flow.Response.Body, flow.Response.Headers, "Response Body") + "\n")
		}
	}

	if flow.Error != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("Error: ") + flow.Error + "\n")
	}

	return b.String()
}

// renderBody safely renders a body that may be binary, compressed, or truncated.
// It never emits raw control bytes that corrupt the TUI.
func renderBody(data []byte, headers map[string][]string, label string) string {
	if len(data) == 0 {
		return "(" + label + ": empty)"
	}
	// Try decompress if Content-Encoding indicates it.
	if enc := headerGet(headers, "content-encoding"); enc != "" {
		enc = strings.ToLower(enc)
		if strings.Contains(enc, "gzip") {
			if dec, err := tryGzip(data); err == nil {
				data = dec
				header := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[decompressed gzip " + fmt.Sprintf("%d", len(data)) + " bytes]")
				return header + "\n" + renderBodyInner(data, headers, label)
			}
		}
	}
	return renderBodyInner(data, headers, label)
}

func renderBodyInner(data []byte, headers map[string][]string, label string) string {
	const previewLimit = 4096
	truncated := false
	if len(data) > previewLimit {
		data = data[:previewLimit]
		truncated = true
	}
	if isBinary(data) {
		ct := headerGet(headers, "content-type")
		if ct != "" {
			ct = " (" + ct + ")"
		}
		header := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(
			fmt.Sprintf("[%s: %d bytes binary%s — hex preview]", label, len(data), ct))
		hexPreview := hexDump(data, 32)
		if truncated {
			hexPreview += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("\n... truncated, full body not shown")
		}
		return header + "\n" + hexPreview
	}
	// Text — ensure valid UTF-8 and no stray controls
	if !utf8.Valid(data) {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(fmt.Sprintf("[%s: %d bytes non-UTF8 binary]", label, len(data))) + "\n" + hexDump(data, 32)
	}
	s := sanitizeForTUI(string(data))
	if truncated {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("\n... truncated to 4KB preview")
	}
	return s
}

func headerGet(headers map[string][]string, key string) string {
	for k, vals := range headers {
		if strings.EqualFold(k, key) && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func isBinary(data []byte) bool {
	if bytes.Contains(data, []byte{0}) {
		return true
	}
	nonPrint := 0
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	for _, b := range sample {
		if b < 32 && b != 9 && b != 10 && b != 13 {
			nonPrint++
		}
		if b == 0xFF || b == 0xFE {
			return true
		}
	}
	return nonPrint > len(sample)/10
}

func sanitizeForTUI(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 32 && r != 9 && r != 10 && r != 13 {
			b.WriteRune('·')
		} else if r == 0x7f {
			b.WriteRune('·')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func tryGzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func hexDump(data []byte, cols int) string {
	var b strings.Builder
	for i := 0; i < len(data); i += cols {
		end := i + cols
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		b.WriteString(fmt.Sprintf("%04x: ", i))
		for _, c := range chunk {
			b.WriteString(fmt.Sprintf("%02x ", c))
		}
		// Pad
		if len(chunk) < cols {
			b.WriteString(strings.Repeat("   ", cols-len(chunk)))
		}
		b.WriteString("|")
		for _, c := range chunk {
			if c >= 32 && c < 127 {
				b.WriteRune(rune(c))
			} else {
				b.WriteRune('.')
			}
		}
		b.WriteString("|\n")
		if b.Len() > 3000 {
			b.WriteString("...\n")
			break
		}
	}
	return b.String()
}
