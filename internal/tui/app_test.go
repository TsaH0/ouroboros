package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TsaH0/ouroboros/internal/model"
	"github.com/TsaH0/ouroboros/internal/scope"
	"github.com/TsaH0/ouroboros/internal/store"
	"github.com/TsaH0/ouroboros/internal/workspace"
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

// scopeWithWildcard creates a scope manager with a default * include rule,
// so test flows are visible in the history table (scope filter is ON).
func scopeWithWildcard(st store.Store) *scope.Manager {
	sc := scope.NewManager(st)
	_, _ = sc.AddRule(context.Background(), scope.Rule{
		Kind: scope.RuleKindHost, Pattern: "*",
		MatchMode: scope.MatchModeWildcard, Action: scope.ActionInclude,
		Enabled: true, Priority: 0,
	})
	return sc
}

// scopeWithHost creates a scope manager with a literal include rule
// for the given host, so the flow is visible in history (scope filter ON).
func scopeWithHost(st store.Store, host string) *scope.Manager {
	sc := scope.NewManager(st)
	_, _ = sc.AddRule(context.Background(), scope.Rule{
		Kind: scope.RuleKindHost, Pattern: host,
		MatchMode: scope.MatchModeLiteral, Action: scope.ActionInclude,
		Enabled: true, Priority: 10,
	})
	return sc
}

func TestHistoryShortcutsOpenScopeAndReturn(t *testing.T) {
	st := store.NewMemoryStore()
	sc := scopeWithWildcard(st)
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

	history := NewHistoryModel(st, nil, 80, 24)
	if len(history.rows) != 1 {
		t.Fatalf("loaded history rows = %d, want 1", len(history.rows))
	}
	if got := history.rows[0][0]; got != "ted-flow" {
		t.Fatalf("loaded flow ID = %q, want ted-flow", got)
	}
}

func TestGlobalKeyOpensHistoryFromScope(t *testing.T) {
	st := store.NewMemoryStore()
	sc := scopeWithWildcard(st)
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
	sc := scopeWithWildcard(st)
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
	sc := scopeWithWildcard(st)
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
	sc := scopeWithWildcard(st)
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
	sc := scopeWithWildcard(st)
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

func TestGlobalImportSelectedFlowAsScope(t *testing.T) {
	st := store.NewMemoryStore()
	flow := &model.Flow{
		ID:          "import-test",
		StartTime:   time.Now(),
		Host:        "import.example.com",
		State:       model.FlowCompleted,
		ScopeStatus: model.ScopeInScope,
		Request:     &model.Message{Method: "GET", URL: "https://import.example.com/"},
	}
	if err := st.SaveFlow(context.Background(), flow); err != nil {
		t.Fatalf("save flow: %v", err)
	}
	sc := scopeWithHost(st, "import.example.com")
	app := NewAppModel(st, nil, sc)

	// History is focused, flow is selected. Press 'i' to import as scope.
	app.Update(appKey("i"))

	// Should have opened scope pane with the new rule.
	panes := app.ws.Layout().Panes()
	if len(panes) != 2 {
		t.Fatalf("pane count = %d, want 2 (history + scope)", len(panes))
	}
	if _, ok := app.ws.FocusedPane().View.(*scopeView); !ok {
		t.Fatalf("focused view = %T, want *scopeView", app.ws.FocusedPane().View)
	}

	// Verify the rule was added (pre-existing scopeWithHost rule + imported).
	rules := sc.Rules()
	if len(rules) != 2 {
		t.Fatalf("rule count = %d, want 2", len(rules))
	}
	found := false
	for _, r := range rules {
		if r.Pattern == "import.example.com" && r.Kind == scope.RuleKindHost {
			found = true
		}
	}
	if !found {
		t.Fatalf("import.example.com rule not found in %v", rules)
	}
}

func TestGlobalImportFromHistoryWithScopeOpen(t *testing.T) {
	st := store.NewMemoryStore()
	flow := &model.Flow{
		ID:          "import-test-2",
		StartTime:   time.Now(),
		Host:        "import2.example.com",
		State:       model.FlowCompleted,
		ScopeStatus: model.ScopeInScope,
		Request:     &model.Message{Method: "GET", URL: "https://import2.example.com/"},
	}
	if err := st.SaveFlow(context.Background(), flow); err != nil {
		t.Fatalf("save flow: %v", err)
	}
	sc := scopeWithHost(st, "import2.example.com")
	app := NewAppModel(st, nil, sc)

	// Open scope pane first (2 panes: history + scope).
	app.Update(appKey("4"))
	if len(app.ws.Layout().Panes()) != 2 {
		t.Fatalf("pane count after scope = %d, want 2", len(app.ws.Layout().Panes()))
	}

	// Focus should be on scope. Press 'i' - should find history pane and import from it.
	app.Update(appKey("i"))

	// Should still have 2 panes, but scope rules updated.
	panes := app.ws.Layout().Panes()
	if len(panes) != 2 {
		t.Fatalf("pane count = %d, want 2", len(panes))
	}
	rules := sc.Rules()
	if len(rules) != 2 {
		t.Fatalf("rule count = %d, want 2", len(rules))
	}
	found := false
	for _, r := range rules {
		if r.Pattern == "import2.example.com" && r.Kind == scope.RuleKindHost {
			found = true
		}
	}
	if !found {
		t.Fatalf("import2.example.com rule not found in %v", rules)
	}
}

func TestColonCommandSaveLoadProject(t *testing.T) {
	// Use temp HOME so projects go to a temp dir.
	t.Setenv("HOME", t.TempDir())

	st := store.NewMemoryStore()
	sc := scope.NewManager(st)
	_, _ = sc.AddRule(context.Background(), scope.Rule{
		Kind: scope.RuleKindHost, Pattern: "test.example.com",
		MatchMode: scope.MatchModeLiteral, Action: scope.ActionInclude,
		Enabled: true, Priority: 10,
	})
	app := NewAppModel(st, nil, sc)

	// Type :w myproject and press enter.
	app.Update(appKey(":"))
	if !app.commandMode {
		t.Fatal("colon should enter command mode")
	}
	app.commandInput.SetValue("w myproject")
	updated, _ := app.Update(appKey("enter"))
	app = updated.(*AppModel)
	if app.commandMode {
		t.Fatal("enter should exit command mode")
	}
	if app.activeProject != "myproject" {
		t.Fatalf("activeProject = %q, want myproject", app.activeProject)
	}

	// Parse the saved file back.
	loaded, err := app.projectStore.Load("myproject")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Pattern != "test.example.com" {
		t.Fatalf("loaded rules = %+v", loaded)
	}

	// Clear rules, then load project with :e.
	sc.ReplaceRules(nil)
	if len(sc.Rules()) != 0 {
		t.Fatal("expected 0 rules after clear")
	}
	app.Update(appKey(":"))
	app.commandInput.SetValue("e myproject")
	updated, _ = app.Update(appKey("enter"))
	app = updated.(*AppModel)
	rules := sc.Rules()
	if len(rules) != 1 || rules[0].Pattern != "test.example.com" {
		t.Fatalf("rules after :e = %+v", rules)
	}
}

func TestColonCommandLsProjects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := store.NewMemoryStore()
	sc := scopeWithWildcard(st)
	app := NewAppModel(st, nil, sc)

	// Save two projects.
	app.Update(appKey(":"))
	app.commandInput.SetValue("w alpha")
	app.Update(appKey("enter"))
	app.Update(appKey(":"))
	app.commandInput.SetValue("w beta")
	app.Update(appKey("enter"))

	// List projects.
	app.Update(appKey(":"))
	app.commandInput.SetValue("ls")
	updated, _ := app.Update(appKey("enter"))
	app = updated.(*AppModel)
	if !strings.Contains(app.activeProject, "alpha") || !strings.Contains(app.activeProject, "beta") {
		t.Fatalf("ls result = %q, want alpha and beta", app.activeProject)
	}
}

func TestColonCommandQuit(t *testing.T) {
	st := store.NewMemoryStore()
	sc := scopeWithWildcard(st)
	app := NewAppModel(st, nil, sc)

	app.Update(appKey(":"))
	app.commandInput.SetValue("q")
	updated, cmd := app.Update(appKey("enter"))
	app = updated.(*AppModel)
	if !app.quitting {
		t.Fatal(":q should set quitting=true")
	}
	if cmd == nil {
		t.Fatal(":q should return tea.Quit command")
	}
}

func TestHistoryScopeToggleKey(t *testing.T) {
	st := store.NewMemoryStore()
	sc := scopeWithWildcard(st)

	// Seed a default allow-all rule like main.go does.
	_, _ = sc.AddRule(context.Background(), scope.Rule{
		Kind: scope.RuleKindHost, Pattern: "*",
		MatchMode: scope.MatchModeWildcard, Action: scope.ActionInclude,
		Enabled: true, Priority: 0,
	})

	// Seed a flow.
	flow := &model.Flow{
		ID:          "scope-toggle-test",
		StartTime:   time.Now(),
		Host:        "toggle.example.com:443",
		State:       model.FlowCompleted,
		ScopeStatus: model.ScopeInScope,
		Request:     &model.Message{Method: "GET", URL: "https://toggle.example.com/"},
	}
	if err := st.SaveFlow(context.Background(), flow); err != nil {
		t.Fatalf("save flow: %v", err)
	}

	app := NewAppModel(st, nil, sc)

	// Host starts in scope (via wildcard). Press 's' to exclude.
	app.Update(appKey("s"))
	status := sc.HostStatus("toggle.example.com")
	if status != model.ScopeOutOfScope {
		t.Fatalf("after first 's', status = %v, want out_of_scope", status)
	}

	// Press 's' again to include. Should go back to in-scope.
	app.Update(appKey("s"))
	status = sc.HostStatus("toggle.example.com")
	if status != model.ScopeInScope {
		t.Fatalf("after second 's', status = %v, want in_scope", status)
	}

	// Press 's' a third time to exclude again.
	app.Update(appKey("s"))
	status = sc.HostStatus("toggle.example.com")
	if status != model.ScopeOutOfScope {
		t.Fatalf("after third 's', status = %v, want out_of_scope", status)
	}

	// Verify only one literal host rule exists (no duplicates).
	rules := sc.Rules()
	hostRules := 0
	for _, r := range rules {
		if r.Kind == scope.RuleKindHost && r.MatchMode == scope.MatchModeLiteral && r.Pattern == "toggle.example.com" {
			hostRules++
		}
	}
	if hostRules != 1 {
		t.Fatalf("literal host rules = %d, want 1 (no duplicates)", hostRules)
	}
}
