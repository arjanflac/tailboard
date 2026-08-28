package clipboard

import (
	"context"
	"strings"
)

// Content represents clipboard data with its MIME type.
type Content struct {
	MimeType string
	Data     []byte
}

// IsText returns true if the content is a text type.
func (c Content) IsText() bool {
	return strings.HasPrefix(c.MimeType, "text/")
}

// Text returns the content as a string. Only meaningful for text types.
func (c Content) Text() string {
	return string(c.Data)
}

// Empty returns true if there is no data.
func (c Content) Empty() bool {
	return len(c.Data) == 0
}

// Clipboard reads and writes the system clipboard with MIME type support.
type Clipboard interface {
	// ReadBest returns the most useful cross-device clipboard content.
	// Priority: image/png > text/plain > text/html. Plain text intentionally
	// wins over an equivalent HTML representation because mobile clients paste
	// a text clip literally and should never expose markup copied by a browser.
	ReadBest() (Content, error)

	// Write sets the clipboard to the given content.
	Write(Content) error

	// Clear removes clipboard contents from the local system clipboard.
	Clear() error
}

// ChangeDetector allows polling agents to avoid reading clipboard contents when
// the platform's cheap change sequence has not moved.
type ChangeDetector interface {
	Changed() (bool, error)
}

// Watcher provides event-driven clipboard change notifications.
type Watcher interface {
	Watch(context.Context) (<-chan struct{}, error)
}

// typePriority defines the preference order for reading clipboard content.
// Higher index = higher priority.
var typePriority = []string{
	"text/html",
	"text/plain",
	"image/png",
}

// bestType picks the highest-priority MIME type from a list of available types.
func bestType(available []string) string {
	set := make(map[string]bool, len(available))
	for _, t := range available {
		// Normalize: "text/plain;charset=utf-8" → "text/plain"
		if idx := strings.IndexByte(t, ';'); idx != -1 {
			t = t[:idx]
		}
		set[strings.TrimSpace(t)] = true
	}

	best := ""
	for _, t := range typePriority {
		if set[t] {
			best = t
		}
	}
	if best == "" && len(available) > 0 {
		// Fall back to text/plain if we have any text type.
		for _, t := range available {
			if strings.HasPrefix(t, "text/") {
				return "text/plain"
			}
		}
	}
	return best
}
