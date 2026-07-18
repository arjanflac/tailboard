//go:build linux

package agent

import (
	"encoding/json"
	"testing"
)

func TestFocusedSwayNodeFindsNestedFloatingWindow(t *testing.T) {
	payload := []byte(`{
		"focused": false,
		"nodes": [{
			"focused": false,
			"floating_nodes": [{
				"focused": true,
				"app_id": "org.keepassxc.KeePassXC",
				"name": "KeePassXC",
				"pid": 42
			}]
		}]
	}`)
	var root swayNode
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatal(err)
	}
	node := focusedSwayNode(root)
	if node == nil || node.AppID != "org.keepassxc.KeePassXC" || node.PID != 42 {
		t.Fatalf("focused node = %+v", node)
	}
}
