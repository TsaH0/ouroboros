package tui

import (
	"context"
	"fmt"
	"strconv"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"ouroboros/internal/model"
	"ouroboros/internal/msg"
	"ouroboros/internal/store"
	"ouroboros/internal/workspace"
)

// HistoryModel wraps the history table as a workspace.View.
type HistoryModel struct {
	id     string
	table  table.Model
	rows   []table.Row
	width  int
	height int
	store  store.Store
}

// NewHistoryModel creates a new HistoryModel.
func NewHistoryModel(st store.Store, width, height int) *HistoryModel {
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Method", Width: 8},
		{Title: "Host", Width: 20},
		{Title: "Path", Width: 30},
		{Title: "Status", Width: 8},
		{Title: "Scope", Width: 6},
		{Title: "Time", Width: 8},
		{Title: "Timestamp", Width: 10},
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
	)
	t.SetWidth(width - 2)
	t.SetHeight(height - 2)

	history := &HistoryModel{
		id:     "history",
		table:  t,
		width:  width,
		height: height,
		store:  st,
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
	return "⏎: detail  r: repeater  a: AI  q: quit"
}
func (m *HistoryModel) IsEditing() bool { return false }

func (m *HistoryModel) Update(mgs tea.Msg) (workspace.View, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.table.SetWidth(v.Width - 2)
		m.table.SetHeight(v.Height - 2)
		return m, nil

	case msg.FlowCompleted:
		m.appendFlow(v.Flow)
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(mgs)
	return m, cmd
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
}

func (m *HistoryModel) appendFlow(flow *model.Flow) {
	if flow == nil {
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
	scopeBadge := "?"
	switch flow.ScopeStatus {
	case model.ScopeInScope:
		scopeBadge = "IN"
	case model.ScopeOutOfScope:
		scopeBadge = "OUT"
	}
	m.rows = append(m.rows, table.Row{
		shortFlowID(flow.ID),
		method,
		flow.Host,
		path,
		strconv.Itoa(status),
		scopeBadge,
		fmt.Sprintf("%dms", flow.Duration.Milliseconds()),
		flow.StartTime.Format("15:04:05"),
	})
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
	return m.table.View()
}

func (m *HistoryModel) Focus() {}
func (m *HistoryModel) Blur()  {}
func (m *HistoryModel) Resize(w, h int) {
	m.width = w
	m.height = h
	m.table.SetWidth(w - 2)
	m.table.SetHeight(h - 2)
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
