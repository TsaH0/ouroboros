package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ouroboros/internal/scope"
	"ouroboros/internal/store"
)

// ScopeModel is the TUI model for managing scope rules.
type ScopeModel struct {
	manager   *scope.Manager
	store     store.Store
	rules     []scope.Rule
	table     table.Model
	input     textinput.Model
	search    textinput.Model
	searching bool
	adding    bool
	addStep   int // 0=action, 1=kind, 2=pattern, 3=priority
	addRule   scope.Rule
	viewport  viewport.Model
	width     int
	height    int
	err       string
	keymap    scopeKeyMap
}

type scopeKeyMap struct {
	back      key.Binding
	add       key.Binding
	delete    key.Binding
	toggle    key.Binding
	search    key.Binding
	importOne key.Binding
	importAll key.Binding
}

func NewScopeModel(mgr *scope.Manager, st store.Store, width, height int) ScopeModel {
	cols := []table.Column{
		{Title: "En", Width: 4},
		{Title: "Action", Width: 8},
		{Title: "Kind", Width: 6},
		{Title: "Pattern", Width: 30},
		{Title: "Priority", Width: 8},
		{Title: "Note", Width: 20},
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
	)
	t.SetWidth(width - 2)
	t.SetHeight(height - 6)

	ti := textinput.New()
	ti.Placeholder = "pattern (e.g. *.example.com)"
	ti.CharLimit = 256

	s := textinput.New()
	s.Placeholder = "search patterns..."
	s.CharLimit = 128

	vp := viewport.New(viewport.WithWidth(max(20, width-2)), viewport.WithHeight(max(3, height-6)))

	rules := mgr.Rules()
	sm := ScopeModel{
		manager:  mgr,
		store:    st,
		rules:    rules,
		table:    t,
		input:    ti,
		search:   s,
		viewport: vp,
		width:    width,
		height:   height,
		keymap: scopeKeyMap{
			back:      key.NewBinding(key.WithKeys("q", "esc")),
			add:       key.NewBinding(key.WithKeys("a")),
			delete:    key.NewBinding(key.WithKeys("d", "x")),
			toggle:    key.NewBinding(key.WithKeys("space", " ", "e")),
			search:    key.NewBinding(key.WithKeys("/")),
			importOne: key.NewBinding(key.WithKeys("i")),
			importAll: key.NewBinding(key.WithKeys("I")),
		},
	}
	sm.refreshTable()
	return sm
}

func (m ScopeModel) Init() tea.Cmd {
	return nil
}

func (m ScopeModel) Update(mgs tea.Msg) (ScopeModel, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.table.SetWidth(v.Width - 2)
		m.table.SetHeight(v.Height - 6)
		m.viewport.SetWidth(max(20, v.Width-2))
		m.viewport.SetHeight(max(3, v.Height-6))
		return m, nil

	case tea.KeyPressMsg:
		if m.searching {
			switch {
			case key.Matches(v, key.NewBinding(key.WithKeys("enter"))):
				m.searching = false
				m.refreshTable()
			case key.Matches(v, key.NewBinding(key.WithKeys("esc"))):
				m.searching = false
				m.search.SetValue("")
				m.refreshTable()
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(mgs)
				m.refreshTable()
				return m, cmd
			}
			return m, nil
		}

		if m.adding {
			return m.updateAddFlow(v)
		}

		switch {
		case key.Matches(v, m.keymap.back):
			return m, func() tea.Msg { return backToListMsg{} }

		case key.Matches(v, m.keymap.add):
			m.adding = true
			m.addStep = 0
			m.err = ""
			m.addRule = scope.Rule{
				Action:    scope.ActionInclude,
				Kind:      scope.RuleKindHost,
				MatchMode: scope.MatchModeWildcard,
				Priority:  10,
				Enabled:   true,
			}
			m.input.SetValue("")
			m.input.Placeholder = "include or exclude"
			m.input.Focus()
			return m, textinput.Blink

		case key.Matches(v, m.keymap.delete):
			row := m.table.SelectedRow()
			if row != nil && len(row) >= 4 {
				pattern := row[3]
				for _, r := range m.rules {
					if r.Pattern == pattern {
						if err := m.manager.DeleteRule(context.Background(), r.ID); err != nil {
							m.err = err.Error()
						}
						break
					}
				}
				m.rules = m.manager.Rules()
				m.refreshTable()
			}

		case key.Matches(v, m.keymap.toggle):
			row := m.table.SelectedRow()
			if row != nil && len(row) >= 4 {
				pattern := row[3]
				for _, r := range m.rules {
					if r.Pattern == pattern {
						if err := m.manager.SetEnabled(context.Background(), r.ID, !r.Enabled); err != nil {
							m.err = err.Error()
						}
						break
					}
				}
				m.rules = m.manager.Rules()
				m.refreshTable()
			}

		case key.Matches(v, m.keymap.search):
			m.searching = true
			m.search.SetValue("")
			m.search.Focus()
			return m, textinput.Blink

		case key.Matches(v, m.keymap.importOne):
			// Import the most recently captured flow's host.
			if m.store == nil {
				m.err = "persistent store is not configured"
				return m, nil
			}
			flows, err := m.store.ListFlows(context.Background())
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			if len(flows) == 0 || flows[len(flows)-1].Host == "" {
				m.err = "no captured flow host available"
				return m, nil
			}
			_, err = m.manager.AddRule(context.Background(), scope.Rule{
				Kind:      scope.RuleKindHost,
				Pattern:   flows[len(flows)-1].Host,
				MatchMode: scope.MatchModeLiteral,
				Action:    scope.ActionInclude,
				Enabled:   true,
				Priority:  10,
			})
			if err != nil {
				m.err = err.Error()
			}
			m.rules = m.manager.Rules()
			m.refreshTable()

		case key.Matches(v, m.keymap.importAll):
			if m.store == nil {
				m.err = "persistent store is not configured"
				return m, nil
			}
			flows, err := m.store.ListFlows(context.Background())
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			seen := make(map[string]bool)
			for _, f := range flows {
				if f.Host == "" || seen[f.Host] {
					continue
				}
				seen[f.Host] = true
				_, err := m.manager.AddRule(context.Background(), scope.Rule{
					Kind:      scope.RuleKindHost,
					Pattern:   f.Host,
					MatchMode: scope.MatchModeLiteral,
					Action:    scope.ActionInclude,
					Enabled:   true,
					Priority:  10,
				})
				if err != nil {
					m.err = err.Error()
				}
			}
			m.rules = m.manager.Rules()
			m.refreshTable()
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(mgs)
	return m, cmd
}

func (m *ScopeModel) updateAddFlow(v tea.KeyPressMsg) (ScopeModel, tea.Cmd) {
	switch {
	case key.Matches(v, key.NewBinding(key.WithKeys("esc"))):
		m.adding = false
		m.input.Blur()
		return *m, nil

	case key.Matches(v, key.NewBinding(key.WithKeys("enter"))):
		switch m.addStep {
		case 0: // action
			val := strings.TrimSpace(m.input.Value())
			if val == "include" || val == "i" {
				m.addRule.Action = scope.ActionInclude
			} else if val == "exclude" || val == "e" {
				m.addRule.Action = scope.ActionExclude
			} else {
				m.err = "type 'include' (i) or 'exclude' (e)"
				return *m, nil
			}
			m.err = ""
			m.addStep = 1
			m.input.SetValue("")
			m.input.Placeholder = "host, path, or url"
			return *m, textinput.Blink

		case 1: // kind
			val := strings.TrimSpace(m.input.Value())
			switch val {
			case "host", "h":
				m.addRule.Kind = scope.RuleKindHost
			case "path", "p":
				m.addRule.Kind = scope.RuleKindPath
			case "url", "u":
				m.addRule.Kind = scope.RuleKindURL
			default:
				m.err = "type 'host' (h), 'path' (p), or 'url' (u)"
				return *m, nil
			}
			m.err = ""
			m.addStep = 2
			m.input.SetValue("")
			m.input.Placeholder = "e.g. *.example.com  or  re:https://..."
			return *m, textinput.Blink

		case 2: // pattern
			pattern := strings.TrimSpace(m.input.Value())
			if pattern == "" {
				m.err = "pattern is required"
				return *m, nil
			}
			m.addRule.Pattern = pattern
			if strings.HasPrefix(pattern, "re:") {
				m.addRule.MatchMode = scope.MatchModeRegex
				m.addRule.Pattern = pattern[3:]
			} else if strings.ContainsAny(pattern, "*?") {
				m.addRule.MatchMode = scope.MatchModeWildcard
			} else {
				m.addRule.MatchMode = scope.MatchModeLiteral
			}
			m.err = ""
			m.addStep = 3
			m.input.SetValue("10")
			m.input.Placeholder = "number (higher = first)"
			return *m, textinput.Blink

		case 3: // priority
			prio := 10
			fmt.Sscanf(m.input.Value(), "%d", &prio)
			m.addRule.Priority = prio

			_, err := m.manager.AddRule(context.Background(), m.addRule)
			if err != nil {
				m.err = err.Error()
			} else {
				m.err = ""
				m.rules = m.manager.Rules()
				m.refreshTable()
			}
			m.adding = false
			m.input.Blur()
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(v)
	return *m, cmd
}

func (m *ScopeModel) refreshTable() {
	rules := m.rules
	if m.searching && m.search.Value() != "" {
		q := strings.ToLower(m.search.Value())
		var filtered []scope.Rule
		for _, r := range rules {
			if strings.Contains(strings.ToLower(r.Pattern), q) ||
				strings.Contains(strings.ToLower(string(r.Kind)), q) ||
				strings.Contains(strings.ToLower(string(r.Action)), q) {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}

	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].CreatedAt.Before(rules[j].CreatedAt)
	})

	var rows []table.Row
	for _, r := range rules {
		enabled := "✓"
		if !r.Enabled {
			enabled = "✗"
		}
		rows = append(rows, table.Row{
			enabled,
			string(r.Action),
			string(r.Kind),
			r.Pattern,
			fmt.Sprintf("%d", r.Priority),
			r.Note,
		})
	}
	m.table.SetRows(rows)
	m.table.UpdateViewport()
}

func (m ScopeModel) View() tea.View {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).
		Width(m.width).Align(lipgloss.Center).
		Render(" Ouroboros - Scope")

	var prompt string
	var body string
	if m.adding {
		switch m.addStep {
		case 0:
			prompt = "Step 1/4 — Action: type 'include' (i) or 'exclude' (e)"
		case 1:
			prompt = "Step 2/4 — Kind: type 'host' (h), 'path' (p), or 'url' (u)"
		case 2:
			prompt = "Step 3/4 — Pattern: type host/path/URL. Use * or ? for wildcard, prefix re: for regex"
		case 3:
			prompt = "Step 4/4 — Priority: type a number (higher = evaluated first)"
		}
		body = lipgloss.JoinVertical(lipgloss.Left,
			prompt,
			"> "+m.input.View(),
			"enter: confirm  esc: cancel",
		)
	} else if m.searching {
		body = lipgloss.JoinVertical(lipgloss.Left,
			"Search: type to filter rules",
			"/ "+m.search.View(),
			"enter: done  esc: cancel",
		)
	} else {
		body = m.table.View()
	}

	var help string
	if !m.adding && !m.searching {
		help = "a:add  d:del  space:toggle  /:search  i:import  I:import all  q:back"
	}

	var errLine string
	if m.err != "" {
		errLine = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("⚠ " + m.err)
	}

	sections := []string{header, body}
	if help != "" {
		sections = append(sections, help)
	}
	if errLine != "" {
		sections = append(sections, errLine)
	}
	return tea.View{Content: lipgloss.JoinVertical(lipgloss.Left, sections...), AltScreen: true}
}
