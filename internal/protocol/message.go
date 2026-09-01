package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// MaxContentSize is the maximum allowed text clipboard size (1 MiB).
const MaxContentSize = 1 << 20

const ProtocolVersion = 3

// ClipItem is the canonical representation of a clipboard entry.
type ClipItem struct {
	Seq       uint64    `json:"seq"`
	Content   string    `json:"content"`
	Hash      string    `json:"hash"`
	Source    string    `json:"source"`
	DeviceID  string    `json:"device_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// WSMessage wraps messages sent over the WebSocket connection.
type WSMessage struct {
	Type string    `json:"type"`
	Item *ClipItem `json:"item,omitempty"`
}

type Capabilities struct {
	HubVersion      string           `json:"hub_version"`
	ProtocolVersion int              `json:"protocol_version"`
	Features        map[string]bool  `json:"features"`
	Limits          CapabilityLimits `json:"limits"`
}

type CapabilityLimits struct {
	MaxClipSize int `json:"max_clip_size"`
}

// HashBytes computes the SHA-256 hex digest of data.
func HashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// HashContent computes the SHA-256 hex digest of a string.
func HashContent(s string) string {
	return HashBytes([]byte(s))
}
