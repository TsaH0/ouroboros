package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TsaH0/ouroboros/internal/recon"
	"github.com/TsaH0/ouroboros/internal/store"
)

func TestAppReconTypingDoesNotBlock(t *testing.T) {
	app := NewAppModel(store.NewMemoryStore(), nil, nil)
	reconModel := NewReconModel(nil, nil, 100, 40)
	app.ws.AddPane(&reconView{ReconModel: &reconModel})

	started := time.Now()
	updatedModel, cmd := app.Update(testKey("e"))
	elapsed := time.Since(started)

	updated := updatedModel.(*AppModel)
	// Find the recon pane and check its target value.
	panes := updated.ws.Layout().Panes()
	var found bool
	for _, p := range panes {
		if rv, ok := p.View.(*reconView); ok {
			if got := rv.target.Value(); got != "e" {
				t.Fatalf("target value = %q, want %q", got, "e")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("recon pane not found in workspace")
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
