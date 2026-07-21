package hub

import (
	"testing"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

func newDeviceTestHub() *Hub {
	h := newTestHub()
	h.devices = make(map[string]protocol.Device)
	return h
}

func TestRemoveDevice(t *testing.T) {
	h := newDeviceTestHub()
	h.RegisterDevice(protocol.RegisterDeviceRequest{DeviceID: "d1", Name: "Phone", Platform: "ios"})

	if !h.RemoveDevice("d1") {
		t.Fatal("expected RemoveDevice to succeed for a registered device")
	}
	if len(h.Devices()) != 0 {
		t.Fatalf("expected empty roster, got %+v", h.Devices())
	}
	if h.RemoveDevice("d1") {
		t.Fatal("expected RemoveDevice to fail for an unknown device")
	}
}

func TestReapStaleDevices(t *testing.T) {
	h := newDeviceTestHub()
	now := time.Now()

	h.devices["fresh-offline"] = protocol.Device{DeviceID: "fresh-offline", Name: "a", LastSeen: now.Add(-time.Hour)}
	h.devices["stale-offline"] = protocol.Device{DeviceID: "stale-offline", Name: "b", LastSeen: now.Add(-staleDeviceTTL - time.Hour)}
	h.devices["stale-online"] = protocol.Device{DeviceID: "stale-online", Name: "c", Online: true, LastSeen: now.Add(-staleDeviceTTL - time.Hour)}

	h.reapStaleDevices(now)

	if _, ok := h.devices["fresh-offline"]; !ok {
		t.Error("recently seen offline device must survive")
	}
	if _, ok := h.devices["stale-offline"]; ok {
		t.Error("device unseen past the TTL must be reaped")
	}
	if _, ok := h.devices["stale-online"]; !ok {
		t.Error("online device must never be reaped regardless of last_seen")
	}
}
