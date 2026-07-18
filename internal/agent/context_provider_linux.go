//go:build linux

package agent

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/thalysguimaraes/cliphub/internal/privacy"
)

type linuxContextProvider struct {
	layer string
}

func (p linuxContextProvider) Layer() string { return p.layer }

func newContextProvider() contextProvider {
	layer := "unavailable"
	switch {
	case os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && commandAvailable("hyprctl"):
		layer = "wayland-hyprland"
	case os.Getenv("SWAYSOCK") != "" && commandAvailable("swaymsg"):
		layer = "wayland-sway"
	case os.Getenv("DISPLAY") != "":
		layer = "x11-ewmh"
	case commandAvailable("xdotool"):
		layer = "xdotool-fallback"
	}
	slog.Info("privacy foreground detector selected", "component", "clipd_privacy", "layer", layer)
	return linuxContextProvider{layer: layer}
}

func (p linuxContextProvider) CurrentContext() (privacy.Context, error) {
	switch p.layer {
	case "wayland-hyprland":
		return hyprlandContext()
	case "wayland-sway":
		return swayContext()
	case "x11-ewmh":
		ctx, err := x11EWMHContext()
		if err == nil {
			return ctx, nil
		}
		if commandAvailable("xdotool") {
			slog.Debug("X11 EWMH detector failed; using xdotool fallback", "component", "clipd_privacy", "error", err)
			return xdotoolContext()
		}
		return privacy.Context{}, err
	case "xdotool-fallback":
		return xdotoolContext()
	default:
		return privacy.Context{}, fmt.Errorf("no foreground-window detector is available")
	}
}

type hyprlandWindow struct {
	Class        string `json:"class"`
	InitialClass string `json:"initialClass"`
	Title        string `json:"title"`
	PID          int    `json:"pid"`
}

func hyprlandContext() (privacy.Context, error) {
	output, err := exec.Command("hyprctl", "activewindow", "-j").Output()
	if err != nil {
		return privacy.Context{}, err
	}
	var window hyprlandWindow
	if err := json.Unmarshal(output, &window); err != nil {
		return privacy.Context{}, err
	}
	appID := firstLinuxValue(window.Class, window.InitialClass)
	return processContext(window.PID, appID, window.Title), nil
}

type swayNode struct {
	Focused          bool       `json:"focused"`
	AppID            string     `json:"app_id"`
	Name             string     `json:"name"`
	PID              int        `json:"pid"`
	Nodes            []swayNode `json:"nodes"`
	FloatingNodes    []swayNode `json:"floating_nodes"`
	WindowProperties *struct {
		Class string `json:"class"`
	} `json:"window_properties"`
}

func swayContext() (privacy.Context, error) {
	output, err := exec.Command("swaymsg", "-t", "get_tree", "-r").Output()
	if err != nil {
		return privacy.Context{}, err
	}
	var root swayNode
	if err := json.Unmarshal(output, &root); err != nil {
		return privacy.Context{}, err
	}
	node := focusedSwayNode(root)
	if node == nil {
		return privacy.Context{}, fmt.Errorf("sway focused window not found")
	}
	appID := node.AppID
	if appID == "" && node.WindowProperties != nil {
		appID = node.WindowProperties.Class
	}
	return processContext(node.PID, appID, node.Name), nil
}

func focusedSwayNode(node swayNode) *swayNode {
	if node.Focused {
		return &node
	}
	for _, child := range append(node.Nodes, node.FloatingNodes...) {
		if focused := focusedSwayNode(child); focused != nil {
			return focused
		}
	}
	return nil
}

func x11EWMHContext() (privacy.Context, error) {
	connection, err := xgb.NewConn()
	if err != nil {
		return privacy.Context{}, err
	}
	defer connection.Close()
	setup := xproto.Setup(connection)
	screen := setup.DefaultScreen(connection)
	activeAtom, err := internAtom(connection, "_NET_ACTIVE_WINDOW")
	if err != nil {
		return privacy.Context{}, err
	}
	active, err := xproto.GetProperty(connection, false, screen.Root, activeAtom, xproto.AtomWindow, 0, 1).Reply()
	if err != nil || len(active.Value) < 4 {
		return privacy.Context{}, fmt.Errorf("read _NET_ACTIVE_WINDOW: %w", err)
	}
	window := xproto.Window(binary.LittleEndian.Uint32(active.Value))
	pidAtom, _ := internAtom(connection, "_NET_WM_PID")
	pidProperty, err := xproto.GetProperty(connection, false, window, pidAtom, xproto.AtomCardinal, 0, 1).Reply()
	if err != nil || len(pidProperty.Value) < 4 {
		return privacy.Context{}, fmt.Errorf("read _NET_WM_PID: %w", err)
	}
	pid := int(binary.LittleEndian.Uint32(pidProperty.Value))
	name := x11WindowText(connection, window, "_NET_WM_NAME")
	class := x11WindowText(connection, window, "WM_CLASS")
	return processContext(pid, class, name), nil
}

func x11WindowText(connection *xgb.Conn, window xproto.Window, property string) string {
	atom, err := internAtom(connection, property)
	if err != nil {
		return ""
	}
	reply, err := xproto.GetProperty(connection, false, window, atom, xproto.GetPropertyTypeAny, 0, 1024).Reply()
	if err != nil {
		return ""
	}
	return strings.Trim(strings.ReplaceAll(string(reply.Value), "\x00", " "), " ")
}

func internAtom(connection *xgb.Conn, name string) (xproto.Atom, error) {
	reply, err := xproto.InternAtom(connection, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	return reply.Atom, nil
}

func xdotoolContext() (privacy.Context, error) {
	pidOutput, err := exec.Command("xdotool", "getactivewindow", "getwindowpid").Output()
	if err != nil {
		return privacy.Context{}, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidOutput)))
	if err != nil {
		return privacy.Context{}, fmt.Errorf("parse active window pid: %w", err)
	}
	nameOutput, _ := exec.Command("xdotool", "getactivewindow", "getwindowclassname").Output()
	return processContext(pid, strings.TrimSpace(string(nameOutput)), ""), nil
}

func processContext(pid int, appID, title string) privacy.Context {
	processName := ""
	if pid > 0 {
		if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
			processName = strings.TrimSpace(string(data))
		}
	}
	appName := firstLinuxValue(appID, processName, title)
	return privacy.Context{
		AppName:     appName,
		BundleID:    appID,
		ProcessName: processName,
	}
}

func firstLinuxValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
