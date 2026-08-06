package searchsploit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sentinel/internal/recon"
)

type fakeRunner struct {
	output []byte
	err    error
	name   string
	args   []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args []string) ([]byte, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	return r.output, r.err
}

func TestProviderUsesPreparedTechnologies(t *testing.T) {
	runner := &fakeRunner{output: []byte(`{"RESULTS_SEARCH":[{"Title":"Apache example exploit","Path":"exploits/1.txt"}]}`)}
	provider := &Provider{Runner: runner}
	provider.Prepare(&recon.ReconSummary{Technologies: []recon.Technology{
		{Name: "Apache", Version: "2.4.41"},
	}})

	findings, err := provider.Run(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if runner.name != "searchsploit" || len(runner.args) != 2 || runner.args[1] != "Apache 2.4.41" {
		t.Fatalf("command = %s %v", runner.name, runner.args)
	}
	if len(findings) != 1 || findings[0].ExploitRef != "exploits/1.txt" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestProviderReportsMissingTechnologies(t *testing.T) {
	provider := &Provider{Runner: &fakeRunner{}}
	_, err := provider.Run(context.Background(), "example.com")
	if err == nil || !strings.Contains(err.Error(), "no detected technologies") {
		t.Fatalf("error = %v", err)
	}
}

func TestProviderReportsCommandFailure(t *testing.T) {
	provider := &Provider{Runner: &fakeRunner{err: errors.New("not found")}}
	provider.Prepare(&recon.ReconSummary{Technologies: []recon.Technology{{Name: "Apache"}}})
	_, err := provider.Run(context.Background(), "example.com")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v", err)
	}
}
