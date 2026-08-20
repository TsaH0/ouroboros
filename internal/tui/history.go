package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ouroboros/internal/model"
	"ouroboros/internal/msg"
	"ouroboros/internal/scope"
	"ouroboros/internal/store"
	"ouroboros/internal/workspace"
)

// colour palette — keep consistent across panes
const (
	colAccent = "39" // cyan-blue
	colGreen  = "34"
	colMuted  = "240"
	colRed    = "196"
	colOrange = "208"
	colYellow = "220"
)

// HistoryModel wraps the history table as a workspace.View.
// ALL captured flows are kept in allFlows — the table rows are a filtered view.
type HistoryModel struct {
	id             string
	table          table.Model
	allFlows       []*model.Flow // source of truth — every flow captured
	rows           []table.Row  // display-only filtered view
	selectedFlowID string       // persists across filter changes
	width          int
	height         int
	store          store.Store
	scopeMgr       *scope.Manager
	scopeFilter    bool   // true = only show in-scope flows
	activePreset   string // display name of active scope preset
	// Inline preset picker state.
	pickingPreset bool
	presetList    []scope.Preset
	presetCursor  int
}

// historyColumns builds table columns that fit within width.
func historyColumns(width int) []table.Column {
	idW, methodW, hostW, pathW, statusW, scopeW, timeW := 8, 6, 22, 0, 6, 5, 6
	avail := width - 2 - (idW + methodW + hostW + statusW + scopeW + timeW)
	if avail > 12 {
		pathW = avail
	} else if avail > 0 {
		hostW += avail // give extra to host
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
		table.Column{Title: "Status", Width: statusW},
		table.Column{Title: "Scope", Width: scopeW},
		table.Column{Title: "Time", Width: timeW},
	)
	return cols
}

// NewHistoryModel creates a new HistoryModel. scopeFilter defaults to false
// (show all traffic) so the user sees everything immediately.
func NewHistoryModel(st store.Store, sc *scope.Manager, width, height int) *HistoryModel {
	cols := historyColumns(max(40, width))
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
	)
	t.SetWidth(max(40, width) - 2)
	t.SetHeight(max(5, height-3))

	presetName := "Global"
	if sc != nil {
		presetName = sc.ActivePresetName()
	}

	m := &HistoryModel{
		id:           "history",
		table:        t,
		width:        width,
		height:       height,
		store:        st,
		scopeMgr:     sc,
		scopeFilter:  false, // show ALL traffic by default; press f to filter
		activePreset: presetName,
	}
	m.reload()
	return m
}

func (m *HistoryModel) ID() string {
	if m.id == "" {
		return "history"
	}
	return m.id
}
func (m *HistoryModel) Title() string    { return "HTTP History" }
func (m *HistoryModel) Init() tea.Cmd    { return nil }
func (m *HistoryModel) IsEditing() bool  { return m.pickingPreset }
func (m *HistoryModel) HelpText() string {
	return "⏎:detail r:repeat s:scope f:filter p:preset C:clear D:wipe q:quit"
}

func (m *HistoryModel) Update(mgs tea.Msg) (workspace.View, tea.Cmd) {
	switch v := mgs.(type) {
	case tea.WindowSizeMsg:
		m.setSize(v.Width, v.Height)
		return m, nil

	case msg.FlowCompleted:
		m.addFlow(v.Flow)
		return m, nil

	case msg.ScopePresetChangedMsg:
		m.activePreset = v.PresetName
		m.applyFilter()
		return m, nil

	case tea.KeyPressMsg:
		if m.pickingPreset {
			return m.handlePresetPicker(v)
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(mgs)
	return m, cmd
}

func (m *HistoryModel) setSize(w, h int) {
	w = max(40, w)
	h = max(5, h)
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
		m.table.SetRows(nil) // clear before column change to prevent panic
		m.table.SetColumns(newCols)
	}
	m.table.SetWidth(w - 2)
	m.table.SetHeight(h - 3)
	if changed {
		m.applyFilter()
	}
	m.width = w
	m.height = h
}

// handlePresetPicker handles keys while the inline preset picker is open.
func (m *HistoryModel) handlePresetPicker(v tea.KeyPressMsg) (workspace.View, tea.Cmd) {
	switch v.String() {
	case "esc", "q", "p":
		m.pickingPreset = false
	case "j", "down":
		if m.presetCursor < len(m.presetList) {
			m.presetCursor++
		}
	case "k", "up":
		if m.presetCursor > 0 {
			m.presetCursor--
		}
	case "enter", " ":
		var id, name string
		if m.presetCursor == 0 {
			id, name = "", "Global"
		} else if idx := m.presetCursor - 1; idx < len(m.presetList) {
			id = m.presetList[idx].ID
			name = m.presetList[idx].Name
		}
		m.pickingPreset = false
		m.activePreset = name
		if m.scopeMgr != nil {
			_ = m.scopeMgr.ActivatePreset(context.Background(), id)
			m.applyFilter()
		}
		return m, func() tea.Msg {
			return msg.ScopePresetChangedMsg{PresetID: id, PresetName: name}
		}
	}
	return m, nil
}

// OpenPresetPicker opens the inline preset picker overlay.
func (m *HistoryModel) OpenPresetPicker() {
	if m.scopeMgr == nil {
		return
	}
	m.presetList = m.scopeMgr.ListPresets()
	activeID := m.scopeMgr.ActivePresetID()
	m.presetCursor = 0
	for i, p := range m.presetList {
		if p.ID == activeID {
			m.presetCursor = i + 1
			break
		}
	}
	m.pickingPreset = true
}

// RefreshScopeBadges recalculates scope column and re-applies filter.
func (m *HistoryModel) RefreshScopeBadges(_ *scope.Manager) { m.applyFilter() }

// reload fetches all flows from store and applies filter.
func (m *HistoryModel) reload() {
	if m.store == nil {
		return
	}
	flows, err := m.store.ListFlows(context.Background())
	if err != nil {
		return
	}
	m.allFlows = flows
	m.applyFilter()
	if len(m.rows) > 0 {
		m.table.SetCursor(0)
	}
}

// ClearScreen clears only the in-memory display (screen), DB untouched.
// Next proxy flows still append; :reload or restart restores DB contents.
func (m *HistoryModel) ClearScreen() {
	m.allFlows = nil
	m.rows = nil
	m.selectedFlowID = ""
	m.table.SetRows(nil)
	m.table.UpdateViewport()
	m.table.SetCursor(0)
}

// addFlow appends a single incoming flow (hot-path, called on every proxy event).
func (m *HistoryModel) addFlow(flow *model.Flow) {
	if flow == nil {
		return
	}
	m.allFlows = append(m.allFlows, flow)
	if !m.scopeFilter || m.isInScope(flow) {
		m.rows = append(m.rows, m.buildRow(flow))
		m.table.SetRows(m.rows)
		m.table.UpdateViewport()
	}
}

// applyFilter rebuilds display rows from allFlows.
func (m *HistoryModel) applyFilter() {
	m.rows = nil
	for _, f := range m.allFlows {
		if m.scopeFilter && !m.isInScope(f) {
			continue
		}
		m.rows = append(m.rows, m.buildRow(f))
	}
	m.table.SetRows(m.rows)
	m.table.UpdateViewport()
}

func (m *HistoryModel) isInScope(f *model.Flow) bool {
	if m.scopeMgr != nil {
		return m.scopeMgr.HostStatus(f.Host) == model.ScopeInScope
	}
	return f.ScopeStatus == model.ScopeInScope
}

// statusColor maps HTTP status codes to terminal colour strings.
func statusColor(code int) string {
	switch {
	case code >= 500:
		return colRed
	case code >= 400:
		return colOrange
	case code >= 300:
		return colYellow
	case code >= 200:
		return colGreen
	default:
		return colMuted
	}
}

func (m *HistoryModel) buildRow(flow *model.Flow) table.Row {
	badge := "·"
	if m.scopeMgr != nil {
		switch m.scopeMgr.HostStatus(flow.Host) {
		case model.ScopeInScope:
			badge = "✓"
		case model.ScopeOutOfScope:
			badge = "✗"
		}
	} else {
		switch flow.ScopeStatus {
		case model.ScopeInScope:
			badge = "✓"
		case model.ScopeOutOfScope:
			badge = "✗"
		}
	}

	status := 0
	path, method := "", ""
	if flow.Request != nil {
		method = flow.Request.Method
		path = flow.Request.URL
	}
	if flow.Response != nil {
		status = flow.Response.StatusCode
	}

	statusStr := ""
	if status > 0 {
		statusStr = strconv.Itoa(status)
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
		case "Status":
			row = append(row, statusStr)
		case "Scope":
			row = append(row, badge)
		case "Time":
			row = append(row, fmt.Sprintf("%dms", flow.Duration.Milliseconds()))
		}
	}
	return row
}

func truncate(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	return s[:w-1] + "…"
}

func shortFlowID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[len(id)-8:]
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m *HistoryModel) View() string {
	if m.pickingPreset {
		return m.renderHeaderBar() + "\n" + m.renderPresetPicker()
	}
	return m.renderHeaderBar() + "\n" + m.table.View()
}

func (m *HistoryModel) renderHeaderBar() string {
	total := len(m.allFlows)
	shown := len(m.rows)

	presetPart := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colAccent)).Bold(true).
		Render("⊙ " + m.activePreset)

	var filterPart string
	if m.scopeFilter {
		filterPart = lipgloss.NewStyle().Foreground(lipgloss.Color(colGreen)).
			Render(fmt.Sprintf(" [filter ON: %d/%d]", shown, total))
	} else {
		filterPart = lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).
			Render(fmt.Sprintf(" [%d flows]", total))
	}

	helpPart := lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).
		Render("  f:filter p:preset r:repeat s:scope C:clear D:wipe")

	return presetPart + filterPart + helpPart
}

func (m *HistoryModel) renderPresetPicker() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colAccent)).
		Render("  Switch Scope Preset") + "\n\n")

	active := ""
	if m.scopeMgr != nil {
		active = m.scopeMgr.ActivePresetID()
	}

	drawRow := func(idx int, label string, id string) {
		cursor := "  "
		if m.presetCursor == idx {
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)).Render("▶ ")
		}
		mark := ""
		if id == active {
			mark = lipgloss.NewStyle().Foreground(lipgloss.Color(colGreen)).Render(" ●")
		}
		b.WriteString(cursor + label + mark + "\n")
	}

	drawRow(0, "Global (all rules)", "")
	for i, p := range m.presetList {
		drawRow(i+1, p.Name, p.ID)
	}

	b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).
		Render("  j/k:move  enter:select  esc:cancel"))
	return b.String()
}

// ── Resize / Focus ────────────────────────────────────────────────────────────

func (m *HistoryModel) Focus() {}
func (m *HistoryModel) Blur()  {}

func (m *HistoryModel) Resize(w, h int) {
	m.setSize(w, h)
}

// ── SelectedFlow ──────────────────────────────────────────────────────────────

// SelectedFlow returns the currently highlighted flow.
// Falls back through: table row → persisted ID → first visible row → first flow.
func (m *HistoryModel) SelectedFlow() *model.Flow {
	// 1. Table's SelectedRow (works at normal terminal sizes).
	if row := m.table.SelectedRow(); row != nil && len(row) > 0 {
		sid := row[0]
		for _, f := range m.allFlows {
			if shortFlowID(f.ID) == sid {
				m.selectedFlowID = f.ID
				return f
			}
		}
	}
	// 2. Persisted ID (survives re-filter that hides the row).
	if m.selectedFlowID != "" {
		for _, f := range m.allFlows {
			if f.ID == m.selectedFlowID {
				return f
			}
		}
	}
	// 3. First visible row (zero-height table in tests).
	if len(m.rows) > 0 {
		sid := m.rows[0][0]
		for _, f := range m.allFlows {
			if shortFlowID(f.ID) == sid {
				m.selectedFlowID = f.ID
				return f
			}
		}
	}
	// 4. Absolute fallback.
	if len(m.allFlows) > 0 {
		m.selectedFlowID = m.allFlows[0].ID
		return m.allFlows[0]
	}
	return nil
}
