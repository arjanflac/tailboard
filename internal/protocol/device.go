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

type TransferFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MIME     string `json:"mime"`
	SHA256   string `json:"sha256"`
	Uploaded int64  `json:"uploaded"`
}

type Transfer struct {
	TransferID string         `json:"transfer_id"`
	FromDevice string         `json:"from_device"`
	ToDevice   string         `json:"to_device"`
	Files      []TransferFile `json:"files"`
	Note       string         `json:"note,omitempty"`
	State      string         `json:"state"`
	CreatedAt  time.Time      `json:"created_at"`
	ExpiresAt  time.Time      `json:"expires_at"`
}

type CreateTransferRequest struct {
	ToDevice string         `json:"to_device"`
	Files    []TransferFile `json:"files"`
	Note     string         `json:"note,omitempty"`
}

type CreateTransferResponse struct {
	Transfer   Transfer `json:"transfer"`
	UploadURLs []string `json:"upload_urls"`
}
