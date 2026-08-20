package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/TsaH0/ouroboros/internal/model"
	"github.com/TsaH0/ouroboros/internal/store"
	"github.com/TsaH0/ouroboros/internal/workspace"
)

// InterceptModel is a queue tab for paused/intercepted flows.
// Shows only flows with State == FlowIntercepted. Select + enter to edit/forward as floating.
type InterceptModel struct {
	id     string
	table  table.Model
	flows  []*model.Flow
	width  int
	height int
	store  store.Store
}

func interceptColumns(width int) []table.Column {
	idW, methodW, hostW, pathW, timeW := 8, 7, 22, 0, 8
	avail := width - 2 - (idW + methodW + hostW + timeW)
	if avail > 12 {
		pathW = avail
	} else if avail > 0 {
		hostW += avail
	}
	cols := []table.Column{
		{Title: "ID", Width: idW},
		{Title: "Method", Width: methodW},
		{Title: "Host", Width: max(10, hostW)},
	}
	if pathW > 0 {
		cols = append(cols, table.Column{Title: "Path", Width: pathW})
	}
	cols = append(cols,
		table.Column{Title: "Time", Width: timeW},
	)
	return cols
}

func NewInterceptModel(st store.Store, width, height int) *InterceptModel {
	cols := interceptColumns(max(40, width))
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
	)
	t.SetWidth(max(40, width) - 2)
	t.SetHeight(max(5, height-3))
	return &InterceptModel{
		id:     "intercept",
		table:  t,
		width:  width,
		height: height,
		store:  st,
	}
}

func (m *InterceptModel) ID() string { return m.id }
func (m *InterceptModel) Title() string { return "Intercept" }
func (m *InterceptModel) Init() tea.Cmd { return nil }
func (m *InterceptModel) IsEditing() bool { return false }
func (m *InterceptModel) HelpText() string { return "enter: edit  f: forward  d: drop  C: clear all  q: back" }
func (m *InterceptModel) Focus() {}
func (m *InterceptModel) Blur() {}

func (m *InterceptModel) Resize(w, h int) { m.setSize(w, h) }

func (m *InterceptModel) setSize(w, h int) {
	w = max(40, w)
	h = max(5, h)
	newCols := interceptColumns(w)
	oldCols := m.table.Columns()
	changed := len(newCols) != len(oldCols)
	if !changed {
		for i, c := range newCols {
			if c.Width != oldCols[i].Width {
				changed = true
				break
			}
		}
	}
	if changed {
		m.table.SetRows(nil)
		m.table.SetColumns(newCols)
	}
	m.table.SetWidth(w - 2)
	m.table.SetHeight(h - 3)
	if changed {
		m.rebuildRows()
	}
	m.width = w
	m.height = h
}

func (m *InterceptModel) Update(msg tea.Msg) (workspace.View, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.setSize(v.Width, v.Height)
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *InterceptModel) View() string {
	count := len(m.flows)
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colAccent)).
		Render(fmt.Sprintf(" Intercept Queue — %d paused", count))
	if count == 0 {
		header += lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Render("  (I toggles intercept, new pauses pile here)")
	} else {
		header += lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Render("  enter: edit (floating)  f:forward  d:drop")
	}
	return header + "\n" + m.table.View()
}

func (m *InterceptModel) AddFlow(flow *model.Flow) {
	if flow == nil {
		return
	}
	// Deduplicate by ID
	for _, f := range m.flows {
		if f.ID == flow.ID {
			return
		}
	}
	m.flows = append(m.flows, flow)
	m.rebuildRows()
}

func (m *InterceptModel) RemoveFlow(id string) {
	filtered := m.flows[:0]
	for _, f := range m.flows {
		if f.ID != id {
			filtered = append(filtered, f)
		}
	}
	m.flows = filtered
	m.rebuildRows()
}

func (m *InterceptModel) ClearAll() {
	m.flows = nil
	m.table.SetRows(nil)
	m.table.UpdateViewport()
}

func (m *InterceptModel) Len() int { return len(m.flows) }

func (m *InterceptModel) rebuildRows() {
	rows := make([]table.Row, 0, len(m.flows))
	for _, f := range m.flows {
		rows = append(rows, m.buildRow(f))
	}
	m.table.SetRows(rows)
	m.table.UpdateViewport()
}

func (m *InterceptModel) buildRow(flow *model.Flow) table.Row {
	method, path := "", ""
	if flow.Request != nil {
		method = flow.Request.Method
		path = flow.Request.URL
		// Strip scheme+host for path column compactness
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			if idx := strings.Index(path[8:], "/"); idx != -1 {
				path = path[8+idx:]
			} else {
				path = "/"
			}
		}
		if len(path) > 60 {
			path = path[:57] + "..."
		}
	}
	cols := m.table.Columns()
	row := make(table.Row, 0, len(cols))
	for _, c := range cols {
		switch c.Title {
		case "ID":
			row = append(row, shortFlowID(flow.ID))
		case "Method":
			row = append(row, method)
		case "Host":
			row = append(row, truncate(flow.Host, c.Width))
		case "Path":
			row = append(row, truncate(path, c.Width))
		case "Time":
			row = append(row, fmt.Sprintf("%dms", flow.Duration.Milliseconds()))
		}
	}
	return row
}

func (m *InterceptModel) SelectedFlow() *model.Flow {
	if row := m.table.SelectedRow(); row != nil && len(row) > 0 {
		sid := row[0]
		for _, f := range m.flows {
			if shortFlowID(f.ID) == sid {
				return f
			}
		}
	}
	if len(m.flows) == 0 {
		return nil
	}
	// Fallback to first row's flow
	if len(m.table.SelectedRow()) == 0 && len(m.flows) > 0 {
		return m.flows[0]
	}
	// Try to match by cursor index
	idx := m.table.Cursor()
	if idx >= 0 && idx < len(m.flows) {
		return m.flows[idx]
	}
	return m.flows[0]
}


