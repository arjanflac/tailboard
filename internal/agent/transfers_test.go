package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

func TestTransferHelpers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, err := transferFileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("hash = %q", hash)
	}
	summary := transferSummary(protocol.Transfer{
		FromDevice: "phone",
		Files:      []protocol.TransferFile{{Size: 2}, {Size: 3}},
	})
	if summary != "2 file(s), 5 bytes from phone" {
		t.Fatalf("summary = %q", summary)
	}
}

func TestRejectsInvalidTransferPolicy(t *testing.T) {
	_, err := New(Config{Clipboard: &fakeClipboard{}, TransferPolicy: "always"})
	if err == nil {
		t.Fatal("expected invalid policy error")
	}
}
