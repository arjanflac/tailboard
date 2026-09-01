package clipboard

import "context"

// Content is the plain text Tailboard reads and writes.
type Content struct {
	Text string
}

// Empty returns true if there is no data.
func (c Content) Empty() bool {
	return c.Text == ""
}

// Clipboard reads and writes plain text only. Taildrop owns files and images.
type Clipboard interface {
	// ReadBest returns the pasteboard's plain-text representation.
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
