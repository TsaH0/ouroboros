package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"ouroboros/internal/model"
	"ouroboros/internal/scope"
	"ouroboros/internal/store"
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
