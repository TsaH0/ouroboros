package tui

import (
	"testing"
	"time"

	"sentinel/internal/store"
)

func TestAppReconTypingDoesNotBlock(t *testing.T) {
	app := NewAppModel(store.NewInMemoryFlowStore(), nil)
	reconModel := NewReconModel(nil, nil, 100, 40)
	app.mode = ModeRecon
	app.recon = &reconModel

	started := time.Now()
	updatedModel, cmd := app.Update(testKey("e"))
	elapsed := time.Since(started)

	updated := updatedModel.(*AppModel)
	if got := updated.recon.target.Value(); got != "e" {
		t.Fatalf("target value = %q, want %q", got, "e")
	}
	if cmd == nil {
		t.Fatal("expected text input command to be returned to Bubble Tea")
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("typing blocked the event loop for %s", elapsed)
	}
}
