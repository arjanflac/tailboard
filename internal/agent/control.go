package agent

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
)

//go:embed control.html
var controlAssets embed.FS

type controlServer struct {
	URL  string
	http *http.Server
}

type controlState struct {
	DeviceID   string              `json:"device_id"`
	NodeName   string              `json:"node_name"`
	HubURL     string              `json:"hub_url"`
	Paused     bool                `json:"paused"`
	Connection string              `json:"connection"` // synced, offline, no-tailscale
	Clip       *protocol.ClipItem  `json:"clip,omitempty"`
	ClipSize   int64               `json:"clip_size,omitempty"`
	Devices    []protocol.Device   `json:"devices"`
	Transfers  []protocol.Transfer `json:"transfers"`
}

type stagedControlFile struct {
	path     string
	manifest protocol.TransferFile
}

func (a *Agent) startControlServer(ctx context.Context) (*controlServer, error) {
	if err := validateLoopbackAddress(a.controlAddr); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", a.controlAddr)
	if err != nil {
		return nil, fmt.Errorf("listen for desktop control surface: %w", err)
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	server := &controlServer{URL: "http://127.0.0.1:" + port}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", server.serveIndex)
	mux.HandleFunc("GET /api/state", a.controlStateHandler)
	mux.HandleFunc("POST /api/pause", a.controlPauseHandler)
	mux.HandleFunc("POST /api/send", a.controlSendHandler)
	mux.HandleFunc("POST /api/reveal", a.controlRevealHandler)
	mux.HandleFunc("POST /api/transfers/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		a.controlTransferActionHandler(ctx, w, r)
	})
	server.http = &http.Server{
		Handler:           sameOrigin(server.URL, mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := server.http.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("desktop control server stopped", "component", "tg-clipd_control", "error", err)
		}
	}()
	return server, nil
}

func (s *controlServer) Shutdown(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *controlServer) serveIndex(w http.ResponseWriter, _ *http.Request) {
	content, err := controlAssets.ReadFile("control.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}

// controlStateHandler always answers quickly, even when the hub is
// unreachable: hub calls run under a short timeout, and failures degrade
// to an empty roster plus a connection status instead of hanging the UI.
// Without this, a dead tailnet route makes /api/state block until the
// client gives up and misreports the engine itself as down.
func (a *Agent) controlStateHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	connection := "synced"
	devices, err := a.client.Devices(ctx)
	if err != nil {
		devices = nil
		if hasTailnetInterface() {
			connection = "offline"
		} else {
			connection = "no-tailscale"
		}
	}

	var transfers []protocol.Transfer
	var clip *protocol.ClipItem
	var clipSize int64
	if connection == "synced" {
		transfers, _ = a.client.Transfers(ctx, a.deviceID, "", "")
		// Best-effort: the popover clip card degrades gracefully without it.
		// Binary payloads are stripped — the popover shows a summary, not
		// the bytes, and state is polled frequently.
		if current, err := a.client.Current(ctx); err == nil && current != nil {
			clipSize = int64(len(current.RawBytes()))
			if len(current.Data) > 0 {
				stripped := *current
				stripped.Data = nil
				clip = &stripped
			} else {
				clip = current
			}
		}
	}

	writeControlJSON(w, http.StatusOK, controlState{
		DeviceID:   a.deviceID,
		NodeName:   a.nodeName,
		HubURL:     a.hubURL,
		Paused:     a.isPaused(),
		Connection: connection,
		Clip:       clip,
		ClipSize:   clipSize,
		Devices:    devices,
		Transfers:  transfers,
	})
}

// hasTailnetInterface reports whether a Tailscale (CGNAT 100.64/10)
// address is present on any local interface — the cheapest local signal
// that Tailscale is up without shelling out to the CLI.
func hasTailnetInterface() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.To4() == nil {
			continue
		}
		ip := ipNet.IP.To4()
		if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
			return true
		}
	}
	return false
}

func (a *Agent) controlPauseHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		controlError(w, err, http.StatusBadRequest)
		return
	}
	a.paused.Store(request.Paused)
	writeControlJSON(w, http.StatusOK, map[string]bool{"paused": request.Paused})
}

func (a *Agent) controlRevealHandler(w http.ResponseWriter, _ *http.Request) {
	if err := openPath(a.downloadDir); err != nil {
		controlError(w, err, http.StatusInternalServerError)
		return
	}
	writeControlJSON(w, http.StatusOK, map[string]bool{"opened": true})
}

func (a *Agent) controlSendHandler(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("to"))
	if target == "" {
		controlError(w, fmt.Errorf("target device is required"), http.StatusBadRequest)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		controlError(w, err, http.StatusBadRequest)
		return
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		controlError(w, fmt.Errorf("at least one file is required"), http.StatusBadRequest)
		return
	}

	staged := make([]stagedControlFile, 0, len(headers))
	defer func() {
		for _, file := range staged {
			_ = os.Remove(file.path)
		}
		_ = r.MultipartForm.RemoveAll()
	}()
	for _, header := range headers {
		source, err := header.Open()
		if err != nil {
			controlError(w, err, http.StatusBadRequest)
			return
		}
		temp, err := os.CreateTemp("", "tg-clipboard-control-*")
		if err != nil {
			source.Close()
			controlError(w, err, http.StatusInternalServerError)
			return
		}
		_, copyErr := io.Copy(temp, source)
		source.Close()
		closeErr := temp.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(temp.Name())
			controlError(w, firstControlError(copyErr, closeErr), http.StatusInternalServerError)
			return
		}
		info, err := os.Stat(temp.Name())
		if err != nil {
			controlError(w, err, http.StatusInternalServerError)
			return
		}
		sum, err := transferFileSHA256(temp.Name())
		if err != nil {
			controlError(w, err, http.StatusInternalServerError)
			return
		}
		name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
		staged = append(staged, stagedControlFile{
			path: temp.Name(),
			manifest: protocol.TransferFile{
				Name: name, Size: info.Size(), MIME: header.Header.Get("Content-Type"), SHA256: sum,
			},
		})
	}

	manifests := make([]protocol.TransferFile, len(staged))
	for i := range staged {
		manifests[i] = staged[i].manifest
	}
	created, err := a.client.CreateTransfer(r.Context(), a.deviceID, protocol.CreateTransferRequest{
		ToDevice: target,
		Files:    manifests,
	})
	if err != nil {
		controlError(w, err, http.StatusBadGateway)
		return
	}
	transfer := created.Transfer
	for index, stagedFile := range staged {
		updated, err := a.uploadControlFile(r.Context(), transfer, index, stagedFile.path)
		if err != nil {
			controlError(w, err, http.StatusBadGateway)
			return
		}
		transfer = *updated
	}
	writeControlJSON(w, http.StatusCreated, transfer)
}

func (a *Agent) uploadControlFile(ctx context.Context, transfer protocol.Transfer, index int, path string) (*protocol.Transfer, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	manifest := transfer.Files[index]
	const chunkSize = int64(8 << 20)
	uploaded := manifest.Uploaded
	for uploaded < manifest.Size || (manifest.Size == 0 && uploaded == 0) {
		length := min(chunkSize, manifest.Size-uploaded)
		updated, err := a.client.UploadTransferFile(
			ctx, a.deviceID, transfer.TransferID, index, uploaded, manifest.Size,
			io.NewSectionReader(file, uploaded, length),
		)
		if err != nil {
			return nil, err
		}
		next := updated.Files[index].Uploaded
		if next <= uploaded && manifest.Size > 0 {
			return nil, fmt.Errorf("upload made no progress at %d bytes", uploaded)
		}
		uploaded = next
		transfer = *updated
		if manifest.Size == 0 {
			break
		}
	}
	return &transfer, nil
}

func (a *Agent) controlTransferActionHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	if action == "decline" {
		transfer, err := a.client.TransferAction(r.Context(), a.deviceID, id, action)
		if err != nil {
			controlError(w, err, http.StatusBadGateway)
			return
		}
		writeControlJSON(w, http.StatusOK, transfer)
		return
	}
	if action != "accept" {
		controlError(w, fmt.Errorf("unsupported transfer action"), http.StatusBadRequest)
		return
	}
	transfers, err := a.client.Transfers(r.Context(), a.deviceID, "receiver", "")
	if err != nil {
		controlError(w, err, http.StatusBadGateway)
		return
	}
	for _, transfer := range transfers {
		if transfer.TransferID == id && transfer.State == "offered" {
			writeControlJSON(w, http.StatusAccepted, map[string]string{"state": "accepting"})
			go a.receiveTransferOnce(ctx, transfer)
			return
		}
	}
	controlError(w, fmt.Errorf("offered transfer not found"), http.StatusNotFound)
}

func validateLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid control address: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("desktop control address must be loopback-only")
	}
	return nil
}

func sameOrigin(baseURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin != baseURL {
			controlError(w, fmt.Errorf("cross-origin request rejected"), http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}

func writeControlJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func controlError(w http.ResponseWriter, err error, status int) {
	writeControlJSON(w, status, map[string]string{"error": err.Error()})
}

func firstControlError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

func openBrowser(url string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Run(); err != nil {
		slog.Debug("failed to open desktop control surface", "component", "tg-clipd_control", "url", url, "error", err)
	}
}

func openPath(path string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", path)
	case "windows":
		command = exec.Command("explorer", path)
	default:
		command = exec.Command("xdg-open", path)
	}
	return command.Start()
}
