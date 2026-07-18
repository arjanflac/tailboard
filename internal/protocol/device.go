package protocol

import "time"

type Device struct {
	DeviceID     string    `json:"device_id"`
	Name         string    `json:"name"`
	Platform     string    `json:"platform"`
	Capabilities []string  `json:"capabilities,omitempty"`
	PublicKey    string    `json:"public_key,omitempty"`
	Online       bool      `json:"online"`
	LastSeen     time.Time `json:"last_seen"`
}

type RegisterDeviceRequest struct {
	DeviceID     string   `json:"device_id"`
	Name         string   `json:"name"`
	Platform     string   `json:"platform"`
	Capabilities []string `json:"capabilities,omitempty"`
	PublicKey    string   `json:"public_key,omitempty"`
}
