package embeddedhub

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

func TestStartServesPersistentHubRole(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stateDir := filepath.Join(t.TempDir(), "hub")
	server, err := Start(ctx, Config{
		Address:  "127.0.0.1:0",
		StateDir: stateDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = server.Shutdown(context.Background())
	})

	resp, err := http.Get(server.URL + "/api/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var capabilities protocol.Capabilities
	if err := json.NewDecoder(resp.Body).Decode(&capabilities); err != nil {
		t.Fatal(err)
	}
	if !capabilities.Features["hub_role"] {
		t.Fatalf("embedded endpoint did not advertise hub role: %+v", capabilities)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "clips.db")); err != nil {
		t.Fatalf("persistent database not created: %v", err)
	}
}
