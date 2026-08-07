package tui

import (
	"context"
	"fmt"
	"strconv"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"ouroboros/internal/model"
	"ouroboros/internal/msg"
	"ouroboros/internal/scope"
	"ouroboros/internal/store"
	"ouroboros/internal/workspace"
)

// HistoryModel wraps the history table as a workspace.View.
type HistoryModel struct {
	id          string
	table       table.Model
	rows        []table.Row
	width       int
	height      int
	store       store.Store
	scopeMgr    *scope.Manager
	scopeFilter bool // true = only show in-scope flows
}

// historyColumns builds table columns that fit within width.
// The Scope column is always visible; less important columns shrink first.
func historyColumns(width int) []table.Column {
	// Essential columns and their minimum widths.
	idW := 8
	methodW := 6
	hostW := 18
	pathW := 20
	statusW := 5
	scopeW := 5
	timeW := 6

	// If narrow, shrink path and host first.
	total := idW + methodW + hostW + pathW + statusW + scopeW + timeW
	if total > width-2 {
		// Drop path entirely on very narrow panes.
		if total-pathW > width-2 {
			pathW = 0
			// Still too wide? shrink host.
			total = idW + methodW + hostW + pathW + statusW + scopeW + timeW
			if total > width-2 {
				hostW = max(8, width-2-(idW+methodW+statusW+scopeW+timeW))
			}
		} else {
			pathW = max(8, width-2-(idW+methodW+hostW+statusW+scopeW+timeW))
		}
	}

	cols := []table.Column{
		{Title: "ID", Width: idW},
		{Title: "Method", Width: methodW},
		{Title: "Host", Width: hostW},
	}
	if pathW > 0 {
		cols = append(cols, table.Column{Title: "Path", Width: pathW})
	}
	cols = append(cols,
		table.Column{Title: "Status", Width: statusW},
		table.Column{Title: "Scope", Width: scopeW},
		table.Column{Title: "Time", Width: timeW},
	)
	return cols
}

// NewHistoryModel creates a new HistoryModel.
func NewHistoryModel(st store.Store, sc *scope.Manager, width, height int) *HistoryModel {
	cols := historyColumns(width)
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
	)
	t.SetWidth(width - 2)
	t.SetHeight(height - 2)

	history := &HistoryModel{
		id:          "history",
		table:       t,
		width:       width,
		height:      height,
		store:       st,
		scopeMgr:    sc,
		scopeFilter: true, // only show in-scope flows by default
	}
	history.reload()
	return history
}

func (m *HistoryModel) ID() string {
	if m.id == "" {
		return "history"
	}
	return m.id
}
func (m *HistoryModel) Title() string { return "History" }
func (m *HistoryModel) Init() tea.Cmd { return nil }
func (m *HistoryModel) HelpText() string {
	return "⏎: detail  r: repeater  a: AI  s: scope  f: filter  q: quit"
}
func (m *HistoryModel) IsEditing() bool { return false }

func (m *HistoryModel) Update(mgs tea.Msg) (workspace.View, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		w := v.Width
		if w < 20 {
			w = 20
		}
		h := v.Height
		if h < 4 {
			h = 4
		}
		newCols := historyColumns(w)
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
			m.rows = nil
			m.table.SetRows(nil)
			m.table.SetColumns(newCols)
		}
		m.table.SetWidth(w - 2)
		m.table.SetHeight(h - 2)
		if changed {
			m.rebuildRows()
		}
		return m, nil

	case msg.FlowCompleted:
		m.appendFlow(v.Flow)
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(mgs)
	return m, cmd
}

// RefreshScopeBadges recalculates the Scope column and re-applies the
// scope filter. Called after scope rules change (e.g. pressing s).
func (m *HistoryModel) RefreshScopeBadges(_ *scope.Manager) {
	m.rebuildRows()
}

// scopeColIndex returns the column index of the Scope column.
func (m *HistoryModel) scopeColIndex() int {
	cols := m.table.Columns()
	for i, c := range cols {
		if c.Title == "Scope" {
			return i
		}
	}
	return -1
}

func (m *HistoryModel) reload() {
	if m.store == nil {
		return
	}
	flows, err := m.store.ListFlows(context.Background())
	if err != nil {
		return
	}
	for _, flow := range flows {
		m.appendFlow(flow)
	}
	if len(m.rows) > 0 {
		m.table.SetCursor(0)
	}
}

// rebuildRows regenerates all rows from the store (used after column changes).
func (m *HistoryModel) rebuildRows() {
	if m.store == nil {
		return
	}
	m.rows = nil
	flows, err := m.store.ListFlows(context.Background())
	if err != nil {
		return
	}
	for _, flow := range flows {
		m.appendFlow(flow)
	}
	// SetRows(nil) in Resize sets cursor to -1; fix it so the first row
	// is selected and SelectedFlow() works.
	if len(m.rows) > 0 {
		m.table.SetCursor(0)
	}
}

func (m *HistoryModel) appendFlow(flow *model.Flow) {
	if flow == nil {
		return
	}

	// Compute scope status first so we can filter.
	scopeBadge := "?"
	var scopeStatus model.ScopeStatus
	if m.scopeMgr != nil {
		scopeStatus = m.scopeMgr.HostStatus(flow.Host)
		switch scopeStatus {
		case model.ScopeInScope:
			scopeBadge = "IN"
		case model.ScopeOutOfScope:
			scopeBadge = "OUT"
		}
	} else {
		scopeStatus = flow.ScopeStatus
		switch scopeStatus {
		case model.ScopeInScope:
			scopeBadge = "IN"
		case model.ScopeOutOfScope:
			scopeBadge = "OUT"
		}
	}

	// If scope filter is on, skip out-of-scope and unknown flows.
	if m.scopeFilter && scopeStatus != model.ScopeInScope {
		return
	}

	status := 0
	path := ""
	method := ""
	if flow.Request != nil {
		method = flow.Request.Method
		path = flow.Request.URL
	}
	if flow.Response != nil {
		status = flow.Response.StatusCode
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
			row = append(row, flow.Host)
		case "Path":
			// Truncate path to fit column width.
			p := path
			if c.Width > 0 && len(p) > c.Width {
				p = p[:c.Width]
			}
			row = append(row, p)
		case "Status":
			row = append(row, strconv.Itoa(status))
		case "Scope":
			row = append(row, scopeBadge)
		case "Time":
			row = append(row, fmt.Sprintf("%dms", flow.Duration.Milliseconds()))
		}
	}
	m.rows = append(m.rows, row)
	m.table.SetRows(m.rows)
	m.table.UpdateViewport()
}

func shortFlowID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[len(id)-8:]
}

func (m *HistoryModel) View() string {
	body := m.table.View()
	if m.scopeFilter {
		body = "  [scope filter ON — f to toggle]\n" + body
	}
	return body
}

func (m *HistoryModel) Focus() {}
func (m *HistoryModel) Blur()  {}

func (m *HistoryModel) Resize(w, h int) {
	m.width = w
	m.height = h
	if w < 20 {
		w = 20
	}
	if h < 4 {
		h = 4
	}
	// Only rebuild if column layout actually changes.
	newCols := historyColumns(w)
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
		m.rows = nil
		m.table.SetRows(nil)
		m.table.SetColumns(newCols)
	}
	m.table.SetWidth(w - 2)
	m.table.SetHeight(h - 2)
	if changed {
		m.rebuildRows()
	}
}

// SelectedFlow returns the flow corresponding to the selected row, or nil.
func (m *HistoryModel) SelectedFlow() *model.Flow {
	row := m.table.SelectedRow()
	if row == nil || len(row) == 0 || m.store == nil {
		return nil
	}
	shortID := row[0]
	flows, err := m.store.ListFlows(context.Background())
	if err != nil {
		return nil
	}
	for _, f := range flows {
		if shortFlowID(f.ID) == shortID {
			return f
		}
	}
	return nil
}
