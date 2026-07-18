//go:build windows

package clipboard

import (
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	cfDIB         = 8
	gmemMoveable  = 0x0002
)

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procRegisterClipboardFormatW   = user32.NewProc("RegisterClipboardFormatW")
	procGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	procGlobalFree                 = kernel32.NewProc("GlobalFree")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
)

type windowsClipboard struct {
	htmlFormat uint32
	pngFormat  uint32
}

func New() (Clipboard, error) {
	html, err := registerFormat("HTML Format")
	if err != nil {
		return nil, err
	}
	pngFormat, err := registerFormat("PNG")
	if err != nil {
		return nil, err
	}
	return &windowsClipboard{htmlFormat: html, pngFormat: pngFormat}, nil
}

func (c *windowsClipboard) ReadBest() (Content, error) {
	if err := openClipboard(); err != nil {
		return Content{}, err
	}
	defer procCloseClipboard.Call()

	switch {
	case formatAvailable(c.pngFormat):
		data, err := readGlobal(c.pngFormat)
		return Content{MimeType: "image/png", Data: data}, err
	case formatAvailable(cfDIB):
		data, err := readGlobal(cfDIB)
		if err != nil {
			return Content{}, err
		}
		pngData, err := dibToPNG(data)
		return Content{MimeType: "image/png", Data: pngData}, err
	case formatAvailable(c.htmlFormat):
		data, err := readGlobal(c.htmlFormat)
		if err != nil {
			return Content{}, err
		}
		return Content{MimeType: "text/html", Data: extractHTMLFragment(data)}, nil
	case formatAvailable(cfUnicodeText):
		data, err := readGlobal(cfUnicodeText)
		if err != nil {
			return Content{}, err
		}
		return Content{MimeType: "text/plain", Data: []byte(decodeUTF16(data))}, nil
	default:
		return Content{}, nil
	}
}

func (c *windowsClipboard) Write(content Content) error {
	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	if result, _, callErr := procEmptyClipboard.Call(); result == 0 {
		return winCallError("EmptyClipboard", callErr)
	}

	switch content.MimeType {
	case "text/plain":
		utf16, err := windows.UTF16FromString(string(content.Data))
		if err != nil {
			return err
		}
		data := make([]byte, len(utf16)*2)
		for i, value := range utf16 {
			binary.LittleEndian.PutUint16(data[i*2:], value)
		}
		return setGlobal(cfUnicodeText, data)
	case "text/html":
		return setGlobal(c.htmlFormat, buildHTMLClipboard(content.Data))
	case "image/png":
		if err := setGlobal(c.pngFormat, content.Data); err != nil {
			return err
		}
		dib, err := pngToDIB(content.Data)
		if err != nil {
			return err
		}
		return setGlobal(cfDIB, dib)
	default:
		return fmt.Errorf("unsupported MIME type for Windows clipboard: %s", content.MimeType)
	}
}

func (c *windowsClipboard) Clear() error {
	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	if result, _, callErr := procEmptyClipboard.Call(); result == 0 {
		return winCallError("EmptyClipboard", callErr)
	}
	return nil
}

func openClipboard() error {
	var callErr error
	for attempt := 0; attempt < 10; attempt++ {
		if result, _, err := procOpenClipboard.Call(0); result != 0 {
			return nil
		} else {
			callErr = err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return winCallError("OpenClipboard", callErr)
}

func registerFormat(name string) (uint32, error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	result, _, callErr := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(ptr)))
	if result == 0 {
		return 0, winCallError("RegisterClipboardFormat", callErr)
	}
	return uint32(result), nil
}

func formatAvailable(format uint32) bool {
	result, _, _ := procIsClipboardFormatAvailable.Call(uintptr(format))
	return result != 0
}

func readGlobal(format uint32) ([]byte, error) {
	handle, _, callErr := procGetClipboardData.Call(uintptr(format))
	if handle == 0 {
		return nil, winCallError("GetClipboardData", callErr)
	}
	size, _, callErr := procGlobalSize.Call(handle)
	if size == 0 {
		return nil, winCallError("GlobalSize", callErr)
	}
	pointer, _, callErr := procGlobalLock.Call(handle)
	if pointer == 0 {
		return nil, winCallError("GlobalLock", callErr)
	}
	defer procGlobalUnlock.Call(handle)
	return append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(pointer)), int(size))...), nil
}

func setGlobal(format uint32, data []byte) error {
	handle, _, callErr := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if handle == 0 {
		return winCallError("GlobalAlloc", callErr)
	}
	pointer, _, callErr := procGlobalLock.Call(handle)
	if pointer == 0 {
		procGlobalFree.Call(handle)
		return winCallError("GlobalLock", callErr)
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(pointer)), len(data)), data)
	procGlobalUnlock.Call(handle)
	if result, _, callErr := procSetClipboardData.Call(uintptr(format), handle); result == 0 {
		procGlobalFree.Call(handle)
		return winCallError("SetClipboardData", callErr)
	}
	return nil
}

func decodeUTF16(data []byte) string {
	values := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		value := binary.LittleEndian.Uint16(data[i:])
		if value == 0 {
			break
		}
		values = append(values, value)
	}
	return syscall.UTF16ToString(values)
}

func winCallError(operation string, err error) error {
	if err == nil || strings.Contains(err.Error(), "operation completed successfully") {
		return fmt.Errorf("%s failed", operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
