//go:build darwin

package clipboard

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include <stdlib.h>
#include "clipboard_darwin.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

type darwinClipboard struct {
	changeMu    sync.Mutex
	changeCount int64
}

// New returns a plain-text Clipboard for macOS.
func New() (Clipboard, error) {
	return &darwinClipboard{changeCount: -1}, nil
}

func (c *darwinClipboard) Changed() (bool, error) {
	count := int64(C.tailboard_pasteboard_change_count())
	if count < 0 {
		return false, fmt.Errorf("read pasteboard change count")
	}

	c.changeMu.Lock()
	defer c.changeMu.Unlock()
	if count == c.changeCount {
		return false, nil
	}
	c.changeCount = count
	return true, nil
}

func (c *darwinClipboard) ReadBest() (Content, error) {
	raw := C.tailboard_pasteboard_copy_text()
	if raw == nil {
		return Content{}, fmt.Errorf("no plain text on clipboard")
	}
	defer C.free(unsafe.Pointer(raw))
	return Content{Text: C.GoString(raw)}, nil
}

func (c *darwinClipboard) Write(content Content) error {
	text := C.CString(content.Text)
	defer C.free(unsafe.Pointer(text))
	if C.tailboard_pasteboard_set_text(text) == 0 {
		return fmt.Errorf("write plain text to pasteboard")
	}
	return nil
}

func (c *darwinClipboard) Clear() error {
	if C.tailboard_pasteboard_clear() == 0 {
		return fmt.Errorf("clear pasteboard")
	}
	return nil
}
