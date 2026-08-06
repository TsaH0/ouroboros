package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"ouroboros/internal/model"
	"ouroboros/internal/scope"
	"ouroboros/internal/store"
	"ouroboros/internal/workspace"
)

func appKey(text string) tea.KeyPressMsg {
	if text == "space" {
		return tea.KeyPressMsg(tea.Key{Code: tea.KeySpace})
	}
	if text == "enter" {
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	}
	r := []rune(text)
	return tea.KeyPressMsg(tea.Key{Text: text, Code: r[0]})
}

func ctrlKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: r, Mod: tea.ModCtrl})
}

func TestHistoryShortcutsOpenScopeAndReturn(t *testing.T) {
	st := store.NewMemoryStore()
	sc := scope.NewManager(st)
	app := NewAppModel(st, nil, sc)

	if _, cmd := app.Update(appKey("4")); cmd != nil {
		t.Fatal("opening scope should not start a command")
	}
	panes := app.ws.Layout().Panes()
	if len(panes) != 2 {
		t.Fatalf("pane count after opening scope = %d, want 2", len(panes))
	}
	if _, ok := app.ws.FocusedPane().View.(*scopeView); !ok {
		t.Fatalf("focused view = %T, want *scopeView", app.ws.FocusedPane().View)
	}

	_, cmd := app.Update(appKey("q"))
	if cmd == nil {
		t.Fatal("scope q should return a back command")
	}
	updated, cmd := app.Update(cmd())
	if cmd != nil {
		t.Fatal("handling back message should not return a command")
	}
	if len(updated.(*AppModel).ws.Layout().Panes()) != 1 {
		t.Fatal("scope pane was not closed by back message")
	}
}

func TestScopeSpaceKeyTogglesRule(t *testing.T) {
	st := store.NewMemoryStore()
	sc := scope.NewManager(st)
	if _, err := sc.AddRule(context.Background(), scope.Rule{
		Kind:      scope.RuleKindHost,
		Pattern:   "example.com",
		MatchMode: scope.MatchModeLiteral,
		Action:    scope.ActionInclude,
		Enabled:   true,
	}); err != nil {
		t.Fatalf("add scope rule: %v", err)
	}
	app := NewAppModel(st, nil, sc)
	app.Update(appKey("4"))
	if _, cmd := app.Update(appKey("space")); cmd != nil {
		t.Fatal("space toggle should not return a command")
	}
	rules := sc.Rules()
	if len(rules) != 1 || rules[0].Enabled {
		t.Fatalf("space did not disable the selected rule: %+v", rules)
	}
}

func TestWorkspaceSplitKeyProducesUniquePane(t *testing.T) {
	app := NewAppModel(store.NewMemoryStore(), nil, nil)
	app.Update(ctrlKey('w'))
	_, cmd := app.Update(appKey("s"))
	if cmd == nil {
		t.Fatal("ctrl+w s should produce a workspace command")
	}
	updated, cmd := app.Update(cmd())
	if cmd != nil {
		t.Fatal("split command should not return another command")
	}
	panes := updated.(*AppModel).ws.Layout().Panes()
	if len(panes) != 2 {
		t.Fatalf("pane count after split = %d, want 2", len(panes))
	}
	if panes[0].ID == panes[1].ID {
		t.Fatalf("split panes share ID %q", panes[0].ID)
	}
}

func TestHistoryLoadsPersistedFlows(t *testing.T) {
	st := store.NewMemoryStore()
	flow := &model.Flow{
		ID:          "persisted-flow",
		StartTime:   time.Now(),
		Host:        "example.com",
		State:       model.FlowCompleted,
		ScopeStatus: model.ScopeInScope,
		Request:     &model.Message{Method: "GET", URL: "https://example.com/"},
	}
	if err := st.SaveFlow(context.Background(), flow); err != nil {
		t.Fatalf("save flow: %v", err)
	}

	history := NewHistoryModel(st, 80, 24)
	if len(history.rows) != 1 {
		t.Fatalf("loaded history rows = %d, want 1", len(history.rows))
	}
	if got := history.rows[0][0]; got != "ted-flow" {
		t.Fatalf("loaded flow ID = %q, want ted-flow", got)
	}
}

func TestGlobalKeyOpensHistoryFromScope(t *testing.T) {
	st := store.NewMemoryStore()
	sc := scope.NewManager(st)
	app := NewAppModel(st, nil, sc)

	// Open scope from history pane.
	app.Update(appKey("4"))
	panes := app.ws.Layout().Panes()
	if len(panes) != 2 {
		t.Fatalf("pane count after scope = %d, want 2", len(panes))
	}
	if _, ok := app.ws.FocusedPane().View.(*scopeView); !ok {
		t.Fatalf("focused view = %T, want *scopeView", app.ws.FocusedPane().View)
	}

	// Press 0 from inside scope to spawn a new history pane.
	// Scope with no input active is NOT editing, so 0 works globally.
	app.Update(appKey("0"))
	panes = app.ws.Layout().Panes()
	if len(panes) != 3 {
		t.Fatalf("pane count after history spawn = %d, want 3", len(panes))
	}
	if _, ok := app.ws.FocusedPane().View.(*HistoryModel); !ok {
		t.Fatalf("focused view = %T, want *HistoryModel", app.ws.FocusedPane().View)
	}
}

func TestReconEditingSuppressesGlobalKeys(t *testing.T) {
	st := store.NewMemoryStore()
	sc := scope.NewManager(st)
	app := NewAppModel(st, nil, sc)

	// Open recon — target input is focused and editing.
	app.Update(appKey("5"))
	if _, ok := app.ws.FocusedPane().View.(*reconView); !ok {
		t.Fatalf("focused view = %T, want *reconView", app.ws.FocusedPane().View)
	}

	// "0" should go to the recon text input, not spawn history.
	app.Update(appKey("0"))
	panes := app.ws.Layout().Panes()
	if len(panes) != 2 {
		t.Fatalf("pane count = %d, want 2 (0 should have been eaten by text input)", len(panes))
	}
}

func TestGlobalKeyOpensScopeFromDetail(t *testing.T) {
	st := store.NewMemoryStore()
	flow := &model.Flow{
		ID:          "test-flow",
		StartTime:   time.Now(),
		Host:        "example.com",
		State:       model.FlowCompleted,
		ScopeStatus: model.ScopeInScope,
		Request:     &model.Message{Method: "GET", URL: "https://example.com/"},
	}
	if err := st.SaveFlow(context.Background(), flow); err != nil {
		t.Fatalf("save flow: %v", err)
	}
	sc := scope.NewManager(st)
	app := NewAppModel(st, nil, sc)

	// Open detail from history.
	app.Update(appKey("enter"))
	panes := app.ws.Layout().Panes()
	if len(panes) != 2 {
		t.Fatalf("pane count after detail = %d, want 2", len(panes))
	}
	if _, ok := app.ws.FocusedPane().View.(*detailView); !ok {
		t.Fatalf("focused view = %T, want *detailView", app.ws.FocusedPane().View)
	}

	// Detail is NOT editing, so 4 opens scope.
	app.Update(appKey("4"))
	panes = app.ws.Layout().Panes()
	if len(panes) != 3 {
		t.Fatalf("pane count after scope spawn = %d, want 3", len(panes))
	}
	if _, ok := app.ws.FocusedPane().View.(*scopeView); !ok {
		t.Fatalf("focused view = %T, want *scopeView", app.ws.FocusedPane().View)
	}
}

func TestCtrlWCQuitsOnLastPane(t *testing.T) {
	st := store.NewMemoryStore()
	sc := scope.NewManager(st)
	app := NewAppModel(st, nil, sc)

	// Only one pane (history). Ctrl+w c should produce AllClosedMsg.
	app.Update(ctrlKey('w'))
	_, cmd := app.Update(appKey("c"))
	if cmd == nil {
		t.Fatal("ctrl+w c on last pane should return a command")
	}

	// The command should emit AllClosedMsg.
	msg := cmd()
	if _, ok := msg.(workspace.AllClosedMsg); !ok {
		t.Fatalf("expected workspace.AllClosedMsg, got %T", msg)
	}

	// AppModel should quit when receiving AllClosedMsg.
	updated, quitCmd := app.Update(msg)
	if quitCmd == nil {
		t.Fatal("AllClosedMsg should trigger tea.Quit")
	}
	if !updated.(*AppModel).quitting {
		t.Fatal("AllClosedMsg should set quitting=true")
	}
}

func TestBackToListQuitsOnLastPane(t *testing.T) {
	st := store.NewMemoryStore()
	flow := &model.Flow{
		ID:          "test-flow",
		StartTime:   time.Now(),
		Host:        "example.com",
		State:       model.FlowCompleted,
		ScopeStatus: model.ScopeInScope,
		Request:     &model.Message{Method: "GET", URL: "https://example.com/"},
	}
	if err := st.SaveFlow(context.Background(), flow); err != nil {
		t.Fatalf("save flow: %v", err)
	}
	sc := scope.NewManager(st)
	app := NewAppModel(st, nil, sc)

	// Open detail from history (2 panes).
	app.Update(appKey("enter"))
	if len(app.ws.Layout().Panes()) != 2 {
		t.Fatalf("pane count = %d, want 2", len(app.ws.Layout().Panes()))
	}

	// Close detail with backToListMsg. Should go back to 1 pane.
	updated, _ := app.Update(backToListMsg{})
	app = updated.(*AppModel)
	if len(app.ws.Layout().Panes()) != 1 {
		t.Fatalf("pane count after first close = %d, want 1", len(app.ws.Layout().Panes()))
	}

	// Close the last pane (history) with backToListMsg. Should quit.
	updated, quitCmd := app.Update(backToListMsg{})
	app = updated.(*AppModel)
	if quitCmd == nil {
		t.Fatal("closing last pane should trigger tea.Quit")
	}
	if !app.quitting {
		t.Fatal("closing last pane should set quitting=true")
	}
}
