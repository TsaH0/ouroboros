package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"sentinel/internal/llm"
	"sentinel/internal/recon"
)

// reconTab selects which data view to display.
type reconTab int

const (
	reconTabSummary reconTab = iota
	reconTabHosts
	reconTabEndpoints
	reconTabTech
	reconTabVulns
	reconTabAI
)

var reconTabNames = []string{"Summary", "Hosts", "Endpoints", "Tech", "Vulns", "AI"}

// reconKeyMsg keys for ReconModel.
type reconKeyMap struct {
	run     key.Binding
	back    key.Binding
	analyze key.Binding
	nextTab key.Binding
	prevTab key.Binding
}

type reconRunMsg struct {
	target string
}

// reconResultMsg carries the recon result back to the TUI.
type reconResultMsg struct {
	summary *recon.ReconSummary
	err     error
}

// reconAIAnalyzeMsg triggers AI analysis of the recon summary.
type reconAIAnalyzeMsg struct {
	summary *recon.ReconSummary
}

// reconAIResultMsg carries the AI analysis result back.
type reconAIResultMsg struct {
	result *llm.ReconAnalysisResult
	err    error
}

// ReconModel is the TUI model for the Recon Intelligence Workspace.
type ReconModel struct {
	engine   *recon.Engine
	analyzer *llm.Analyzer
	target   textinput.Model
	summary  *recon.ReconSummary
	aiResult *llm.ReconAnalysisResult
	progress  *recon.ProgressUpdate
	loading   bool
	aiLoading bool
	tab       reconTab
	spinner  spinner.Model
	viewport viewport.Model
	keymap   reconKeyMap
	width    int
	height   int
	err      error
}

// NewReconModel creates a new ReconModel.
func NewReconModel(engine *recon.Engine, analyzer *llm.Analyzer, width, height int) ReconModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ti := textinput.New()
	ti.Placeholder = "example.com"
	ti.Focus()
	ti.CharLimit = 256

	vp := viewport.New(viewport.WithWidth(max(20, width-2)), viewport.WithHeight(max(3, height-6)))

	return ReconModel{
		engine:   engine,
		analyzer: analyzer,
		target:   ti,
		spinner:  sp,
		viewport: vp,
		width:    width,
		height:   height,
		tab:      reconTabSummary,
		keymap: reconKeyMap{
			run:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "run recon")),
			back:    key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back")),
			analyze: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "AI analyze")),
			nextTab: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
			prevTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "prev tab")),
		},
	}
}

func (m ReconModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m ReconModel) Update(mgs tea.Msg) (ReconModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch v := mgs.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.target.SetWidth(max(20, v.Width-4))
		m.viewport.SetWidth(max(20, v.Width-2))
		m.viewport.SetHeight(max(3, v.Height-6))
		return m, nil

	case tea.KeyPressMsg:
		// If loading, only allow quit.
		if m.loading || m.aiLoading {
			if key.Matches(v, m.keymap.back) {
				return m, func() tea.Msg { return backToListMsg{} }
			}
			return m, nil
		}

		// If we have results, tab navigation and AI analysis are active.
		if m.summary != nil {
			switch {
			case key.Matches(v, m.keymap.back):
				return m, func() tea.Msg { return backToListMsg{} }
			case key.Matches(v, m.keymap.analyze):
				m.aiLoading = true
				return m, tea.Batch(
					m.spinner.Tick,
					func() tea.Msg { return reconAIAnalyzeMsg{summary: m.summary} },
				)
			case key.Matches(v, m.keymap.nextTab):
				m.tab = (m.tab + 1) % reconTabAI
				if m.tab == reconTabAI && m.aiResult == nil {
					m.tab = reconTabSummary
				}
				m.refreshViewport()
				return m, nil
			case key.Matches(v, m.keymap.prevTab):
				if m.tab == 0 {
					m.tab = reconTabAI - 1
				} else {
					m.tab--
				}
				if m.tab == reconTabAI && m.aiResult == nil {
					m.tab = reconTabSummary
				}
				m.refreshViewport()
				return m, nil
			}
			// Let viewport handle j/k scrolling.
			break
		}

		// No results yet — target input is active.
		switch {
		case key.Matches(v, m.keymap.back):
			return m, func() tea.Msg { return backToListMsg{} }
		case key.Matches(v, m.keymap.run):
			target := strings.TrimSpace(m.target.Value())
			if target == "" {
				return m, nil
			}
			m.loading = true
			m.progress = nil
			return m, tea.Batch(
				m.spinner.Tick,
				func() tea.Msg { return reconRunMsg{target: target} },
			)
		}

		// Delegate to textinput for typing.
		var ticmd tea.Cmd
		m.target, ticmd = m.target.Update(mgs)
		cmds = append(cmds, ticmd)

	case reconResultMsg:
		m.loading = false
		if v.err != nil {
			m.err = v.err
			m.viewport.SetContent(fmt.Sprintf("Recon failed: %v\n\nPress q to go back.", v.err))
		} else {
			m.summary = v.summary
			m.tab = reconTabSummary
			m.refreshViewport()
		}
		return m, nil

	case recon.ProgressUpdate:
		m.progress = &v
		return m, nil


	case reconAIResultMsg:
		m.aiLoading = false
		if v.err != nil {
			m.viewport.SetContent(fmt.Sprintf("AI analysis failed: %v\n\nPress q to go back.", v.err))
		} else {
			m.aiResult = v.result
			m.tab = reconTabAI
			m.refreshViewport()
		}
		return m, nil
	}

	// Spinner and viewport updates.
	var spcmd, vpcmd tea.Cmd
	m.spinner, spcmd = m.spinner.Update(mgs)
	m.viewport, vpcmd = m.viewport.Update(mgs)
	cmds = append(cmds, spcmd, vpcmd)

	return m, tea.Batch(cmds...)
}

func (m *ReconModel) refreshViewport() {
	content := m.renderTab()
	m.viewport.SetContent(content)
}

func (m ReconModel) View() tea.View {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).
		Width(m.width).Align(lipgloss.Center).
		Render(" Sentinel — Recon Intelligence")

	var tabBar string
	if m.summary != nil {
		var tabs []string
		for i, name := range reconTabNames {
			style := lipgloss.NewStyle().Padding(0, 2)
			if reconTab(i) == m.tab {
				style = style.Bold(true).Foreground(lipgloss.Color("39")).Background(lipgloss.Color("235"))
			} else {
				style = style.Foreground(lipgloss.Color("240"))
			}
			tabs = append(tabs, style.Render(name))
		}
		tabBar = lipgloss.JoinHorizontal(lipgloss.Left, tabs...)
	}

	var body string
	switch {
	case m.loading:
		progTxt := "running providers..."
		if m.progress != nil {
			progTxt = fmt.Sprintf("running %s...", m.progress.Provider)
		}
		body = m.spinner.View() + " " + progTxt
	case m.aiLoading:
		body = m.spinner.View() + " AI analyzing..."
	case m.summary == nil:
		input := lipgloss.NewStyle().Margin(1, 0).Render("Target: " + m.target.View())
		body = lipgloss.JoinVertical(lipgloss.Left, input, "\nenter: run recon  q: back")
	default:
		body = m.viewport.View()
	}

	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
		"enter: run  tab/shift+tab: tabs  a: AI analyze  q: back")

	sections := []string{header}
	if tabBar != "" {
		sections = append(sections, tabBar)
	}
	sections = append(sections, body, footer)

	return tea.View{Content: lipgloss.JoinVertical(lipgloss.Left, sections...), AltScreen: true}
}

func (m ReconModel) renderTab() string {
	if m.summary == nil {
		return ""
	}
	switch m.tab {
	case reconTabSummary:
		return renderReconSummary(m.summary)
	case reconTabHosts:
		return renderReconHosts(m.summary)
	case reconTabEndpoints:
		return renderReconEndpoints(m.summary)
	case reconTabTech:
		return renderReconTech(m.summary)
	case reconTabVulns:
		return renderReconVulns(m.summary)
	case reconTabAI:
		return renderReconAI(m.aiResult)
	}
	return ""
}

func renderReconSummary(s *recon.ReconSummary) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Recon Summary"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Target:       %s\n", s.Target))
	b.WriteString(fmt.Sprintf("Hosts:        %d\n", s.HostCount()))
	b.WriteString(fmt.Sprintf("Endpoints:    %d (%d interesting)\n", s.EndpointCount(), len(s.InterestingEndpoints())))
	b.WriteString(fmt.Sprintf("Technologies: %d\n", s.TechCount()))
	b.WriteString(fmt.Sprintf("Vulns:        %d\n", s.VulnCount()))
	b.WriteString(fmt.Sprintf("Created:      %s\n", s.CreatedAt.Format("2006-01-02 15:04:05")))

	interesting := s.InterestingEndpoints()
	if len(interesting) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Top Interesting Endpoints"))
		b.WriteString("\n\n")
		sorted := s.SortedEndpoints()
		max := 20
		if len(sorted) < max {
			max = len(sorted)
		}
		for i := 0; i < max; i++ {
			e := sorted[i]
			b.WriteString(fmt.Sprintf("  [%s] %s\n", e.Category, e.URL))
		}
		if len(sorted) > max {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(sorted)-max))
		}
	}
	return b.String()
}

func renderReconHosts(s *recon.ReconSummary) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Hosts (%d)", s.HostCount())))
	b.WriteString("\n\n")
	if len(s.Hosts) == 0 {
		b.WriteString("No hosts discovered.\n")
		return b.String()
	}
	for _, h := range s.Hosts {
		srcs := make([]string, len(h.Sources))
		for i, src := range h.Sources {
			srcs[i] = string(src)
		}
		b.WriteString(fmt.Sprintf("  %s  [%s]\n", h.Hostname, strings.Join(srcs, ", ")))
	}
	return b.String()
}

func renderReconEndpoints(s *recon.ReconSummary) string {
	var b strings.Builder
	sorted := s.SortedEndpoints()
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Endpoints (%d)", s.EndpointCount())))
	b.WriteString("\n\n")
	if len(sorted) == 0 {
		b.WriteString("No endpoints discovered.\n")
		return b.String()
	}
	for _, e := range sorted {
		b.WriteString(fmt.Sprintf("  [%s] %s\n", e.Category, e.URL))
	}
	return b.String()
}

func renderReconTech(s *recon.ReconSummary) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Technologies (%d)", s.TechCount())))
	b.WriteString("\n\n")
	if len(s.Technologies) == 0 {
		b.WriteString("No technologies detected.\n")
		return b.String()
	}
	for _, t := range s.Technologies {
		ver := t.Version
		if ver == "" {
			ver = "?"
		}
		b.WriteString(fmt.Sprintf("  %s %s  (host: %s, src: %s)\n", t.Name, ver, t.Host, t.Source))
	}
	return b.String()
}

func renderReconVulns(s *recon.ReconSummary) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Potential Vulnerabilities (%d)", s.VulnCount())))
	b.WriteString("\n\n")
	if len(s.Vulnerabilities) == 0 {
		b.WriteString("No known vulnerabilities found.\n")
		return b.String()
	}
	for _, v := range s.Vulnerabilities {
		cve := v.CVE
		if cve == "" {
			cve = "N/A"
		}
		b.WriteString(fmt.Sprintf("  %s: %s\n", cve, v.Title))
		if v.ExploitRef != "" {
			b.WriteString(fmt.Sprintf("    exploit: %s\n", v.ExploitRef))
		}
	}
	return b.String()
}

func renderReconAI(result *llm.ReconAnalysisResult) string {
	if result == nil {
		return "No AI analysis yet. Press 'a' to analyze.\n"
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("AI Attack Surface Prioritization"))
	b.WriteString("\n\n")

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Summary"))
	b.WriteString("\n")
	b.WriteString(result.Summary)
	b.WriteString("\n\n")

	if len(result.HighPriority) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("High Priority"))
		b.WriteString("\n")
		for _, item := range result.HighPriority {
			b.WriteString(fmt.Sprintf("  ! %s\n", item))
		}
		b.WriteString("\n")
	}

	if len(result.InterestingHosts) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Interesting Hosts"))
		b.WriteString("\n")
		for _, h := range result.InterestingHosts {
			b.WriteString(fmt.Sprintf("  - %s\n", h))
		}
		b.WriteString("\n")
	}

	if len(result.InterestingEndpoints) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Interesting Endpoints"))
		b.WriteString("\n")
		for _, e := range result.InterestingEndpoints {
			b.WriteString(fmt.Sprintf("  - %s\n", e))
		}
		b.WriteString("\n")
	}

	if len(result.RecommendedOrder) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Recommended Testing Order"))
		b.WriteString("\n")
		for i, step := range result.RecommendedOrder {
			b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, step))
		}
		b.WriteString("\n")
	}

	if len(result.InterestingPatterns) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Interesting Patterns"))
		b.WriteString("\n")
		for _, p := range result.InterestingPatterns {
			b.WriteString(fmt.Sprintf("  - %s\n", p))
		}
		b.WriteString("\n")
	}

	if len(result.Reasoning) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Reasoning"))
		b.WriteString("\n")
		for _, r := range result.Reasoning {
			b.WriteString(fmt.Sprintf("  - %s\n", r))
		}
	}

	return b.String()
}

// Suppress unused import warnings — context is used by app.go recon handlers.
var _ = context.Background