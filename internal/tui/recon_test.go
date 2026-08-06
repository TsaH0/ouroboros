package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"ouroboros/internal/recon"
	"ouroboros/internal/store"
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

func TestRenderReconSummaryShowsProviderFailures(t *testing.T) {
	summary := &recon.ReconSummary{
		Target: "example.com",
		Providers: []recon.ProviderStatus{
			{
				Name:   "subfinder",
				Status: "error",
				Error:  "executable file not found in PATH",
			},
		},
	}

	rendered := renderReconSummary(summary, errors.New("all recon providers failed"))
	for _, want := range []string{
		"Recon could not collect results",
		"Provider Status",
		"[error] subfinder",
		"executable file not found in PATH",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered summary missing %q:\n%s", want, rendered)
		}
	}
}
