//go:build windows

package clipboard

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	cfDIB         = 8
	gmemMoveable  = 0x0002

	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmClipboardUpdate = 0x031D
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
	procGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	procAddClipboardFormatListener = user32.NewProc("AddClipboardFormatListener")
	procRemoveClipboardListener    = user32.NewProc("RemoveClipboardFormatListener")
	procRegisterClassExW           = user32.NewProc("RegisterClassExW")
	procUnregisterClassW           = user32.NewProc("UnregisterClassW")
	procCreateWindowExW            = user32.NewProc("CreateWindowExW")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procDefWindowProcW             = user32.NewProc("DefWindowProcW")
	procGetMessageW                = user32.NewProc("GetMessageW")
	procTranslateMessage           = user32.NewProc("TranslateMessage")
	procDispatchMessageW           = user32.NewProc("DispatchMessageW")
	procPostMessageW               = user32.NewProc("PostMessageW")
	procPostQuitMessage            = user32.NewProc("PostQuitMessage")
	procGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	procGlobalFree                 = kernel32.NewProc("GlobalFree")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
	procGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")

	clipboardWatchers sync.Map
)

type windowsPoint struct {
	X int32
	Y int32
}

type windowsMessage struct {
	Window  windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   windowsPoint
	Private uint32
}

type windowsClassEx struct {
	Size        uint32
	Style       uint32
	WindowProc  uintptr
	ClassExtra  int32
	WindowExtra int32
	Instance    windows.Handle
	Icon        windows.Handle
	Cursor      windows.Handle
	Background  windows.Handle
	MenuName    *uint16
	ClassName   *uint16
	IconSmall   windows.Handle
}

type windowsClipboard struct {
	htmlFormat   uint32
	pngFormat    uint32
	lastSequence uint32
}

func (c *windowsClipboard) Changed() (bool, error) {
	sequence, _, callErr := procGetClipboardSequenceNumber.Call()
	if sequence == 0 {
		return false, winCallError("GetClipboardSequenceNumber", callErr)
	}
	current := uint32(sequence)
	if current == c.lastSequence {
		return false, nil
	}
	c.lastSequence = current
	return true, nil
}

func (c *windowsClipboard) Watch(ctx context.Context) (<-chan struct{}, error) {
	events := make(chan struct{}, 1)
	ready := make(chan error, 1)
	window := make(chan windows.Handle, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		className, err := windows.UTF16PtrFromString(fmt.Sprintf("TGClipboardClipboardWatcher-%d", os.Getpid()))
		if err != nil {
			ready <- err
			close(events)
			return
		}
		instance, _, callErr := procGetModuleHandleW.Call(0)
		if instance == 0 {
			ready <- winCallError("GetModuleHandleW", callErr)
			close(events)
			return
		}
		class := windowsClassEx{
			Size:       uint32(unsafe.Sizeof(windowsClassEx{})),
			WindowProc: windows.NewCallback(clipboardWindowProc),
			Instance:   windows.Handle(instance),
			ClassName:  className,
		}
		atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
		if atom == 0 {
			ready <- winCallError("RegisterClassExW", callErr)
			close(events)
			return
		}
		defer procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), instance)

		// HWND_MESSAGE (-3) creates an invisible message-only window.
		hwnd, _, callErr := procCreateWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(className)),
			0,
			0, 0, 0, 0,
			^uintptr(2),
			0,
			instance,
			0,
		)
		if hwnd == 0 {
			ready <- winCallError("CreateWindowExW", callErr)
			close(events)
			return
		}
		clipboardWatchers.Store(hwnd, events)
		defer clipboardWatchers.Delete(hwnd)

		result, _, callErr := procAddClipboardFormatListener.Call(hwnd)
		if result == 0 {
			procDestroyWindow.Call(hwnd)
			ready <- winCallError("AddClipboardFormatListener", callErr)
			close(events)
			return
		}
		defer procRemoveClipboardListener.Call(hwnd)

		window <- windows.Handle(hwnd)
		ready <- nil
		var message windowsMessage
		for {
			result, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
			if int32(result) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
		}
		close(events)
	}()

	if err := <-ready; err != nil {
		return nil, err
	}
	hwnd := <-window
	go func() {
		<-ctx.Done()
		procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
	}()
	return events, nil
}

func clipboardWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmClipboardUpdate:
		if raw, ok := clipboardWatchers.Load(hwnd); ok {
			select {
			case raw.(chan struct{}) <- struct{}{}:
			default:
			}
		}
		return 0
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	default:
		result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
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
