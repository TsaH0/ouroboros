package tui

import (
	tea "charm.land/bubbletea/v2"

	"ouroboros/internal/workspace"
)

// reconView wraps a ReconModel as a workspace.View.
type reconView struct {
	*ReconModel
	id string
}

func (v *reconView) ID() string {
	if v.id != "" {
		return v.id
	}
	return "recon-" + v.target.Value()
}
func (v *reconView) Title() string { return "Recon" }
func (v *reconView) View() string  { return v.ReconModel.View().Content }
func (v *reconView) HelpText() string {
	return "enter: run  tab/S-tab: tabs  a: AI analyze  q: back"
}
func (v *reconView) IsEditing() bool {
	return v.ReconModel.summary == nil && !v.ReconModel.loading && !v.ReconModel.scopeBlocked
}
func (v *reconView) Focus() {}
func (v *reconView) Blur()  {}
func (v *reconView) Resize(w, h int) {
	v.ReconModel.width = w
	v.ReconModel.height = h
	v.ReconModel.target.SetWidth(max(20, w-4))
	v.ReconModel.viewport.SetWidth(max(20, w-2))
	v.ReconModel.viewport.SetHeight(max(3, h-6))
}
func (v *reconView) Update(mgs tea.Msg) (workspace.View, tea.Cmd) {
	updated, cmd := v.ReconModel.Update(mgs)
	return &reconView{ReconModel: &updated, id: v.id}, cmd
}

// repeaterView wraps a RepeaterModel as a workspace.View.
type repeaterView struct {
	*RepeaterModel
	id string
}

func (v *repeaterView) ID() string {
	if v.id != "" {
		return v.id
	}
	return "repeater-" + v.flow.ID
}
func (v *repeaterView) Title() string { return "Repeater" }
func (v *repeaterView) View() string  { return v.RepeaterModel.View().Content }
func (v *repeaterView) HelpText() string {
	return "s/enter: send  i: edit  j/k: navigate  q: back"
}
func (v *repeaterView) IsEditing() bool { return v.RepeaterModel.editing }
func (v *repeaterView) Focus()          {}
func (v *repeaterView) Blur()           {}
func (v *repeaterView) Resize(w, h int) {
	v.RepeaterModel.width = w
	v.RepeaterModel.height = h
}
func (v *repeaterView) Update(mgs tea.Msg) (workspace.View, tea.Cmd) {
	updated, cmd := v.RepeaterModel.Update(mgs)
	return &repeaterView{RepeaterModel: &updated, id: v.id}, cmd
}

// llmView wraps a LLMModel as a workspace.View.
type llmView struct {
	*LLMModel
	id string
}

func (v *llmView) ID() string {
	if v.id != "" {
		return v.id
	}
	if v.flow != nil {
		return "llm-" + v.flow.ID
	}
	return "llm-bulk"
}
func (v *llmView) Title() string { return "AI Analysis" }
func (v *llmView) View() string  { return v.LLMModel.View().Content }
func (v *llmView) HelpText() string {
	if v.LLMModel.bulkKind == LLMViewBulk {
		return "a: analyze all  q: back"
	}
	return "a: analyze  q: back"
}
func (v *llmView) IsEditing() bool { return false }
func (v *llmView) Focus()          {}
func (v *llmView) Blur()           {}
func (v *llmView) Resize(w, h int) {
	v.LLMModel.width = w
	v.LLMModel.height = h
}
func (v *llmView) Update(mgs tea.Msg) (workspace.View, tea.Cmd) {
	updated, cmd := v.LLMModel.Update(mgs)
	return &llmView{LLMModel: &updated, id: v.id}, cmd
}

// scopeView wraps a ScopeModel as a workspace.View.
type scopeView struct {
	*ScopeModel
	id string
}

func (v *scopeView) ID() string {
	if v.id != "" {
		return v.id
	}
	return "scope"
}
func (v *scopeView) Title() string { return "Scope" }
func (v *scopeView) View() string  { return v.ScopeModel.View().Content }
func (v *scopeView) HelpText() string {
	return "a: add  d: delete  space: toggle  /: search  i: import  I: import all  q: back"
}
func (v *scopeView) IsEditing() bool { return v.ScopeModel.adding || v.ScopeModel.searching }
func (v *scopeView) Focus()          {}
func (v *scopeView) Blur()           {}
func (v *scopeView) Resize(w, h int) {
	v.ScopeModel.width = w
	v.ScopeModel.height = h
}
func (v *scopeView) Update(mgs tea.Msg) (workspace.View, tea.Cmd) {
	updated, cmd := v.ScopeModel.Update(mgs)
	return &scopeView{ScopeModel: &updated, id: v.id}, cmd
}

// detailView wraps a DetailModel as a workspace.View.
type detailView struct {
	*DetailModel
	id string
}

func (v *detailView) ID() string {
	if v.id != "" {
		return v.id
	}
	return "detail-" + v.flow.ID
}
func (v *detailView) Title() string { return "Detail" }
func (v *detailView) View() string  { return v.DetailModel.View().Content }
func (v *detailView) Focus()        {}
func (v *detailView) HelpText() string {
	return "f: forward  d: drop  a: analyze  q: back"
}
func (v *detailView) IsEditing() bool { return false }
func (v *detailView) Blur()           {}
func (v *detailView) Resize(w, h int) {
	v.DetailModel.width = w
	v.DetailModel.height = h
	v.DetailModel.viewport.SetWidth(max(20, w-2))
	v.DetailModel.viewport.SetHeight(max(3, h-4))
}
func (v *detailView) Update(mgs tea.Msg) (workspace.View, tea.Cmd) {
	updated, cmd := v.DetailModel.Update(mgs)
	return &detailView{DetailModel: &updated, id: v.id}, cmd
}

// Compile-time interface checks.
var (
	_ workspace.View = (*reconView)(nil)
	_ workspace.View = (*repeaterView)(nil)
	_ workspace.View = (*llmView)(nil)
	_ workspace.View = (*scopeView)(nil)
	_ workspace.View = (*detailView)(nil)
	_ workspace.View = (*HistoryModel)(nil)
)
