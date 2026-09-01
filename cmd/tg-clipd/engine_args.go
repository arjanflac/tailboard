package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const engineArgumentsFileName = "engine-arguments.json"

var bundledEngineDefaults = []string{"--embed-hub"}

// configuredRuntimeArgs loads the per-user arguments written by the macOS app.
// The SMAppService login-item bundle stays immutable inside the signed app;
// user-specific settings belong in Application Support.
func configuredRuntimeArgs() ([]string, error) {
	path := engineArgumentsPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if runningFromAppBundle() {
			return append([]string(nil), bundledEngineDefaults...), nil
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read engine arguments: %w", err)
	}
	var args []string
	if err := json.Unmarshal(data, &args); err != nil {
		return nil, fmt.Errorf("decode engine arguments: %w", err)
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, '\x00') {
			return nil, fmt.Errorf("decode engine arguments: argument contains NUL")
		}
	}
	return args, nil
}

func writeConfiguredRuntimeArgs(args []string) error {
	path := engineArgumentsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create engine configuration directory: %w", err)
	}
	data, err := json.MarshalIndent(args, "", "  ")
	if err != nil {
		return fmt.Errorf("encode engine arguments: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".engine-arguments-*.json")
	if err != nil {
		return fmt.Errorf("create engine configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure engine configuration: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write engine configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close engine configuration: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish engine configuration: %w", err)
	}
	return nil
}

func engineArgumentsPath() string {
	if override := strings.TrimSpace(os.Getenv("TG_CLIPBOARD_ARGUMENTS_FILE")); override != "" {
		return override
	}
	return filepath.Join(defaultStateDir(), engineArgumentsFileName)
}

func runningFromAppBundle() bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	path := filepath.ToSlash(executable)
	return strings.Contains(path, ".app/Contents/")
}
