package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thalysguimaraes/cliphub/internal/agent"
)

type stubAgentRunner struct {
	runErr error
}

func TestRunEmbeddedHubUsesLocalEndpoint(t *testing.T) {
	t.Setenv("CLIPHUB_HUB", "")
	origNewAgent := newAgent
	var captured agent.Config
	newAgent = func(cfg agent.Config) (agentRunner, error) {
		captured = cfg
		return stubAgentRunner{}, nil
	}
	t.Cleanup(func() {
		newAgent = origNewAgent
	})

	err := run(context.Background(), []string{
		"-embed-hub",
		"-embed-hub-addr", "127.0.0.1:0",
		"-state-dir", filepath.Join(t.TempDir(), "agent"),
		"-node", "embedded-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(captured.HubURL, "http://127.0.0.1:") {
		t.Fatalf("agent did not use embedded hub endpoint: %q", captured.HubURL)
	}
}

func (s stubAgentRunner) Run(context.Context) error {
	return s.runErr
}

func TestRunMainClipboardInitFailure(t *testing.T) {
	origNewAgent := newAgent
	newAgent = func(cfg agent.Config) (agentRunner, error) {
		return nil, &agent.ClipboardInitError{Err: errors.New("clipboard unavailable")}
	}
	t.Cleanup(func() {
		newAgent = origNewAgent
	})

	var logs bytes.Buffer
	origLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(origLogger)
	})

	exitCode := runMain([]string{"-hub", "http://127.0.0.1:1", "-node", "test-node"})
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, "clipboard init failed") {
		t.Fatalf("expected clipboard init log, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "clipboard unavailable") {
		t.Fatalf("expected underlying error in log, got %q", logOutput)
	}
}
