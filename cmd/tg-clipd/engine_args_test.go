package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestConfiguredRuntimeArgsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine-arguments.json")
	t.Setenv("TG_CLIPBOARD_ARGUMENTS_FILE", path)
	want := []string{
		"--embed-hub",
		"--node", "Test Mac",
	}
	if err := writeConfiguredRuntimeArgs(want); err != nil {
		t.Fatalf("writeConfiguredRuntimeArgs() error = %v", err)
	}
	got, err := configuredRuntimeArgs()
	if err != nil {
		t.Fatalf("configuredRuntimeArgs() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("configuredRuntimeArgs() = %#v, want %#v", got, want)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat engine arguments: %v", err)
		}
		if gotMode := info.Mode().Perm(); gotMode != 0o600 {
			t.Fatalf("engine arguments mode = %#o, want 0600", gotMode)
		}
	}
}

func TestConfiguredRuntimeArgsRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine-arguments.json")
	t.Setenv("TG_CLIPBOARD_ARGUMENTS_FILE", path)
	if err := os.WriteFile(path, []byte(`{"embed_hub":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := configuredRuntimeArgs(); err == nil {
		t.Fatal("configuredRuntimeArgs() expected invalid JSON error")
	}
}
