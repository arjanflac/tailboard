package deviceid

import (
	"path/filepath"
	"testing"
)

func TestLoadOrCreateIsStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "device-id")
	first, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !valid(first) {
		t.Fatalf("expected stable UUID, got %q then %q", first, second)
	}
}
