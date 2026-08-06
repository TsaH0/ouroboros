package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"sentinel/internal/model"
)

func testKey(s string) tea.KeyPressMsg {
	if s == "esc" {
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	}
	if s == "enter" {
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	}
	if len(s) == 1 {
		r := []rune(s)[0]
		return tea.KeyPressMsg(tea.Key{Text: s, Code: r})
	}
	return tea.KeyPressMsg(tea.Key{Text: s})
}

func TestRepeaterBackKeyReturnsToHistory(t *testing.T) {
	m := NewRepeaterModel(&model.Flow{}, 100, 40)
	_, cmd := m.Update(testKey("q"))
	if cmd == nil {
		t.Fatal("expected q to produce back command")
	}
	if _, ok := cmd().(backToListMsg); !ok {
		t.Fatalf("expected backToListMsg, got %T", cmd())
	}
}

func TestRepeaterSendKeyWorksInNormalMode(t *testing.T) {
	flow := &model.Flow{Request: &model.Message{Method: "GET", URL: "http://example.test/"}}
	m := NewRepeaterModel(flow, 100, 40)
	_, cmd := m.Update(testKey("s"))
	if cmd == nil {
		t.Fatal("expected s to produce send command")
	}
	msg, ok := cmd().(repeaterSendMsg)
	if !ok {
		t.Fatalf("expected repeaterSendMsg, got %T", cmd())
	}
	if msg.edits.Method != "GET" || msg.edits.URL != "http://example.test/" {
		t.Fatalf("unexpected edits: %#v", msg.edits)
	}
}

func TestRepeaterVimNavigationAndInsertMode(t *testing.T) {
	m := NewRepeaterModel(&model.Flow{}, 100, 40)
	m2, cmd := m.Update(testKey("j"))
	if cmd != nil {
		t.Fatal("navigation should not produce command")
	}
	if m2.focusIdx != 1 {
		t.Fatalf("focusIdx = %d, want 1", m2.focusIdx)
	}
	m3, _ := m2.Update(testKey("i"))
	if !m3.editing {
		t.Fatal("i should enter insert mode")
	}
	m4, _ := m3.Update(testKey("esc"))
	if m4.editing {
		t.Fatal("esc should leave insert mode")
	}
}
