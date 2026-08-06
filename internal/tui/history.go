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

	return &HistoryModel{
		table:  t,
		width:  width,
		height: height,
		store:  st,
	}
}

func (m *HistoryModel) ID() string    { return "history" }
func (m *HistoryModel) Title() string { return "History" }
func (m *HistoryModel) Init() tea.Cmd { return nil }

func (m *HistoryModel) Update(mgs tea.Msg) (workspace.View, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.table.SetWidth(v.Width - 2)
		m.table.SetHeight(v.Height - 2)
		return m, nil

	case msg.FlowCompleted:
		flow := v.Flow
		status := 0
		path := ""
		method := ""
		host := flow.Host
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
		row := table.Row{
			flow.ID[len(flow.ID)-8:],
			method,
			host,
			path,
			strconv.Itoa(status),
			scopeBadge,
			fmt.Sprintf("%dms", flow.Duration.Milliseconds()),
			flow.StartTime.Format("15:04:05"),
		}
		m.rows = append(m.rows, row)
		m.table.SetRows(m.rows)
		m.table.UpdateViewport()
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(mgs)
	return m, cmd
}

func (m *HistoryModel) View() string {
	return m.table.View()
}

func (m *HistoryModel) Focus()    {}
func (m *HistoryModel) Blur()     {}
func (m *HistoryModel) Resize(w, h int) {
	m.width = w
	m.height = h
	m.table.SetWidth(w - 2)
	m.table.SetHeight(h - 2)
}

// SelectedFlow returns the flow corresponding to the selected row, or nil.
func (m *HistoryModel) SelectedFlow() *model.Flow {
	row := m.table.SelectedRow()
	if row == nil {
		return nil
	}
	shortID := row[0]
	flows, _ := m.store.ListFlows(context.Background())
	for _, f := range flows {
		if len(f.ID) >= 8 && f.ID[len(f.ID)-8:] == shortID {
			return f
		}
	}
	return nil
}
