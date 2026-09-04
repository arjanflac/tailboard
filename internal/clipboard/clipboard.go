package clipboard

import "errors"

var ErrNoText = errors.New("clipboard does not contain standalone text")

// Clipboard exposes only literal text. File URLs, file promises, images, and
// values marked concealed or transient are deliberately invisible here.
type Clipboard interface {
	ReadText() (string, error)
	WriteText(string) error
	Clear() error
	Changed() (bool, error)
}
