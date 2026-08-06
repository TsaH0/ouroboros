package workspace

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// testView is a minimal View implementation for testing.
type testView struct {
	id    string
	title string
	data  string
}

func (v *testView) ID() string                     { return v.id }
func (v *testView) Title() string                  { return v.title }
func (v *testView) Init() tea.Cmd                  { return nil }
func (v *testView) Update(tea.Msg) (View, tea.Cmd) { return v, nil }
func (v *testView) View() string                   { return v.data }
func (v *testView) Focus()                         {}
func (v *testView) Blur()                          {}
func (v *testView) Resize(w, h int)                {}
func (v *testView) HelpText() string               { return "" }
func (v *testView) IsEditing() bool                { return false }

func newTestView(id, title string) *testView {
	return &testView{id: id, title: title, data: id + " content"}
}

func TestManager_AddPane(t *testing.T) {
	m := NewManager()
	v := newTestView("p1", "Pane 1")
	p := m.AddPane(v)
	if p == nil {
		t.Fatal("AddPane returned nil")
	}
	if m.FocusedPane() == nil {
		t.Fatal("no focused pane")
	}
	if m.FocusedPane().ID != "p1" {
		t.Fatalf("focused = %s, want p1", m.FocusedPane().ID)
	}
}

func TestManager_FocusNext(t *testing.T) {
	m := NewManager()
	m.AddPane(newTestView("p1", "Pane 1"))
	m.SplitHSplit(newTestView("p2", "Pane 2"))

	if m.FocusedPane().ID != "p2" {
		t.Fatalf("focused = %s, want p2 (new split)", m.FocusedPane().ID)
	}

	m.FocusNext()
	if m.FocusedPane().ID != "p1" {
		t.Fatalf("after FocusNext: focused = %s, want p1", m.FocusedPane().ID)
	}

	m.FocusNext()
	if m.FocusedPane().ID != "p2" {
		t.Fatalf("after second FocusNext: focused = %s, want p2", m.FocusedPane().ID)
	}
}

func TestManager_CloseFocused(t *testing.T) {
	m := NewManager()
	m.AddPane(newTestView("p1", "Pane 1"))
	m.SplitHSplit(newTestView("p2", "Pane 2"))

	m.CloseFocused() // closes p2 (focused)
	if m.FocusedPane() == nil {
		t.Fatal("no focused pane after close")
	}
	if m.FocusedPane().ID != "p1" {
		t.Fatalf("focused = %s, want p1", m.FocusedPane().ID)
	}

	// Close last pane.
	m.CloseFocused()
	if m.FocusedPane() != nil {
		t.Fatal("expected nil focused after closing last pane")
	}
}

func TestManager_CloseAllButFocused(t *testing.T) {
	m := NewManager()
	m.AddPane(newTestView("p1", "Pane 1"))
	m.SplitHSplit(newTestView("p2", "Pane 2"))
	m.SplitVSplit(newTestView("p3", "Pane 3"))

	m.CloseAllButFocused()
	panes := m.layout.Panes()
	if len(panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(panes))
	}
	if panes[0].ID != "p3" {
		t.Fatalf("remaining pane = %s, want p3", panes[0].ID)
	}
}

func TestManager_Equalize(t *testing.T) {
	m := NewManager()
	m.AddPane(newTestView("p1", "Pane 1"))
	m.SplitHSplit(newTestView("p2", "Pane 2"))

	// Set uneven weight.
	m.layout.Weight = 0.75
	m.Equalize()
	if m.layout.Weight != 0.5 {
		t.Fatalf("weight = %f, want 0.5", m.layout.Weight)
	}
}

func TestManager_Resize(t *testing.T) {
	m := NewManager()
	m.AddPane(newTestView("p1", "Pane 1"))
	m.SplitHSplit(newTestView("p2", "Pane 2"))

	m.Resize(100, 50)
	panes := m.layout.Panes()
	if len(panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(panes))
	}
	// Each pane should have non-zero dimensions.
	for _, p := range panes {
		if p.Width == 0 || p.Height == 0 {
			t.Fatalf("pane %s has zero dimensions: %dx%d", p.ID, p.Width, p.Height)
		}
	}
}

func TestManager_KeyboardRouting(t *testing.T) {
	m := NewManager()
	v1 := newTestView("p1", "Pane 1")
	v2 := newTestView("p2", "Pane 2")
	m.AddPane(v1)
	m.SplitHSplit(v2)

	// FocusPrev should move to p1.
	m.FocusPrev()
	if m.FocusedPane().ID != "p1" {
		t.Fatalf("after FocusPrev: focused = %s, want p1", m.FocusedPane().ID)
	}

	// FocusNext should move to p2.
	m.FocusNext()
	if m.FocusedPane().ID != "p2" {
		t.Fatalf("after FocusNext: focused = %s, want p2", m.FocusedPane().ID)
	}
}

func TestLayout_Panes(t *testing.T) {
	p1 := &Pane{ID: "p1"}
	p2 := &Pane{ID: "p2"}
	p3 := &Pane{ID: "p3"}

	layout := NewHSplit(
		NewLeaf(p1),
		NewVSplit(NewLeaf(p2), NewLeaf(p3), 0.5),
		0.5,
	)

	panes := layout.Panes()
	if len(panes) != 3 {
		t.Fatalf("expected 3 panes, got %d", len(panes))
	}
}

func TestLayout_FindPaneByID(t *testing.T) {
	p1 := &Pane{ID: "p1"}
	p2 := &Pane{ID: "p2"}
	layout := NewHSplit(NewLeaf(p1), NewLeaf(p2), 0.5)

	if layout.FindPaneByID("p1") != p1 {
		t.Fatal("FindPaneByID p1 failed")
	}
	if layout.FindPaneByID("p2") != p2 {
		t.Fatal("FindPaneByID p2 failed")
	}
	if layout.FindPaneByID("nonexistent") != nil {
		t.Fatal("FindPaneByID nonexistent should return nil")
	}
}
