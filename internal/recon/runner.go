package recon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// DefaultCommandRunner is the default implementation of CommandRunner
// using os/exec with context support.
type DefaultCommandRunner struct{}

// Run executes the given command and returns its stdout.
func (DefaultCommandRunner) Run(ctx context.Context, name string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s failed: %s: %s", name, exitErr.Error(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("%s failed: %w", name, err)
	}
	return out, nil
}

// ensureBinary checks if a binary exists in PATH.
func ensureBinary(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s not found in PATH: %w", name, err)
	}
	return nil
}

// signalForKill returns the signal that likely killed the process.
func signalForKill(err error) bool {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.Signaled()
		}
	}
	return false
}
