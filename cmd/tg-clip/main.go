package main

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/thalysguimaraes/tg-clipboard/internal/clipboard"
	"github.com/thalysguimaraes/tg-clipboard/internal/deviceid"
	"github.com/thalysguimaraes/tg-clipboard/internal/discover"
	"github.com/thalysguimaraes/tg-clipboard/internal/hubclient"
	"github.com/thalysguimaraes/tg-clipboard/internal/protocol"
)

// version is injected via ldflags in reproducible release builds.
var version = "dev"

var hub *hubclient.Client
var localDeviceID string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Allow --hub flag anywhere.
	hubURL := os.Getenv("TG_CLIPBOARD_HUB")
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "--hub" && i+1 < len(args) {
			hubURL = args[i+1]
			args = append(args[:i], args[i+2:]...)
			break
		}
		if strings.HasPrefix(arg, "--hub=") {
			hubURL = strings.TrimPrefix(arg, "--hub=")
			args = append(args[:i], args[i+1:]...)
			break
		}
	}

	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	if hubURL == "" {
		resolver := discover.NewResolver(discover.DefaultConfig())
		if url, err := resolver.HubURL(ctx); err == nil {
			hubURL = url
		} else {
			hubURL = "http://localhost:8080"
		}
	}

	var err error
	hub, err = hubclient.New(hubclient.Config{BaseURL: hubURL})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	localDeviceID, err = deviceid.LoadOrCreate(filepath.Join(defaultStateDir(), "device-id"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	hostName, _ := os.Hostname()
	_, _ = hub.RegisterDevice(ctx, protocol.RegisterDeviceRequest{
		DeviceID: localDeviceID, Name: hostName, Platform: runtime.GOOS,
		Capabilities: []string{"clipboard", "transfers", "direct-fetch-source"},
	})

	switch args[0] {
	case "get":
		err = cmdGet(ctx, args[1:])
	case "put":
		err = cmdPut(ctx, args[1:])
	case "history":
		err = cmdHistory(ctx, args[1:])
	case "status":
		err = cmdStatus(ctx)
	case "devices":
		err = cmdDevices(ctx)
	case "send":
		err = cmdSend(ctx, args[1:])
	case "transfers":
		err = cmdTransfers(ctx, args[1:])
	case "receive":
		err = cmdReceive(ctx, args[1:])
	case "clear":
		err = cmdClear(ctx, args[1:])
	case "pause":
		err = cmdPause()
	case "resume":
		err = cmdResume()
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: tg-clip <command> [args]

Commands:
  get [-o file]              Print current clip (or save binary to file)
  put [text]                 Send text to clipboard (reads stdin if no args)
  put --file path            Send a file (MIME auto-detected from extension)
  put --mime type [text]     Send with explicit MIME type
  history [-n N]             Show clipboard history
  status                     Show hub status
  devices                    List registered devices and online state
  send FILE --to DEVICE      Send a file or directory to a registered device
       [--direct] [--wait]   Prefer direct fetch; optionally wait for receipt
  send FILE --resume ID      Resume an interrupted outgoing transfer
  transfers                  List incoming and outgoing transfers
  receive --id ID [--to DIR] Accept and download a transfer
  clear [--local]            Clear hub clipboard/history (and optionally this machine's clipboard)
  pause                      Pause clipboard sync
  resume                     Resume clipboard sync

Flags:
  --hub URL        Hub URL (default: auto-discovered, $TG_CLIPBOARD_HUB, or localhost)

Environment:
  TG_CLIPBOARD_HUB        Explicit hub URL override
  TG_CLIPBOARD_HOSTNAME   Tailnet hostname used for auto-discovery (default: tg-clipboard)
`)
}

func cmdSend(ctx context.Context, args []string) error {
	var path, target, resumeID string
	var preferDirect, wait bool
	for i := 0; i < len(args); i++ {
		if args[i] == "--to" && i+1 < len(args) {
			target = args[i+1]
			i++
		} else if args[i] == "--resume" && i+1 < len(args) {
			resumeID = args[i+1]
			i++
		} else if args[i] == "--direct" {
			preferDirect = true
		} else if args[i] == "--wait" {
			wait = true
		} else if path == "" {
			path = args[i]
		}
	}
	if path == "" || (target == "" && resumeID == "") {
		return fmt.Errorf("usage: tg-clip send FILE (--to DEVICE | --resume ID)")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	sendName := filepath.Base(path)
	cleanup := func() {}
	if info.IsDir() {
		sendName += ".tar"
		path, cleanup, err = archiveDirectory(path)
		if err != nil {
			return err
		}
		defer cleanup()
		info, err = os.Stat(path)
		if err != nil {
			return err
		}
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file or directory", path)
	}
	sum, err := hashFile(path)
	if err != nil {
		return err
	}
	var transfer protocol.Transfer
	if resumeID != "" {
		outgoing, err := hub.Transfers(ctx, localDeviceID, "sender", "uploading")
		if err != nil {
			return err
		}
		found := false
		for _, candidate := range outgoing {
			if candidate.TransferID == resumeID {
				transfer = candidate
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("uploading transfer %s not found", resumeID)
		}
		if len(transfer.Files) != 1 || transfer.Files[0].Size != info.Size() ||
			!strings.EqualFold(transfer.Files[0].SHA256, sum) {
			return fmt.Errorf("local payload does not match transfer %s", resumeID)
		}
	} else {
		targetDevice, err := resolveDeviceRecord(ctx, target)
		if err != nil {
			return err
		}
		request := protocol.CreateTransferRequest{
			ToDevice: targetDevice.DeviceID,
			Files: []protocol.TransferFile{{
				Name: sendName, Size: info.Size(), MIME: mimeFromExt(filepath.Ext(sendName)), SHA256: sum,
			}},
		}
		var directServer *directTransferServer
		if preferDirect && deviceSupportsDirect(targetDevice) {
			resolver := discover.NewResolver(discover.DefaultConfig())
			tailnetIP, resolveErr := resolver.SelfIP(ctx)
			if resolveErr == nil {
				directServer, err = startDirectTransferServer(ctx, net.JoinHostPort(tailnetIP, "0"), []string{path})
				if err == nil {
					request.Mode = "direct"
					request.DirectURL = directServer.URL
					request.DirectToken = directServer.Token
				}
			}
			if resolveErr != nil || err != nil {
				fmt.Fprintln(os.Stderr, "direct fetch unavailable; falling back to hub spool")
				directServer = nil
			}
		} else if preferDirect {
			fmt.Fprintln(os.Stderr, "target is offline or lacks direct-fetch support; falling back to hub spool")
		}
		if directServer != nil {
			defer directServer.Shutdown(context.Background())
		}
		created, err := hub.CreateTransfer(ctx, localDeviceID, request)
		if err != nil {
			if request.Mode != "direct" {
				return err
			}
			fmt.Fprintln(os.Stderr, "direct offer was rejected; falling back to hub spool")
			request.Mode, request.DirectURL, request.DirectToken = "", "", ""
			created, err = hub.CreateTransfer(ctx, localDeviceID, request)
			if err != nil {
				return err
			}
		}
		transfer = created.Transfer
		if request.Mode == "direct" {
			if transfer.Mode != "direct" {
				_, _ = hub.TransferAction(context.Background(), localDeviceID, transfer.TransferID, "cancel")
				fmt.Fprintln(os.Stderr, "hub does not support direct fetch; falling back to hub spool")
				created, err = hub.CreateTransfer(ctx, localDeviceID, protocol.CreateTransferRequest{
					ToDevice: request.ToDevice, Files: request.Files,
				})
				if err != nil {
					return err
				}
				transfer = created.Transfer
			} else {
				fmt.Printf("offered %s (%d bytes) directly as %s\n", sendName, info.Size(), transfer.TransferID)
				return waitForTransfer(ctx, transfer.TransferID)
			}
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	const chunkSize = int64(8 << 20)
	uploaded := transfer.Files[0].Uploaded
	startedAt := time.Now()
	startedOffset := uploaded
	for uploaded < info.Size() || (info.Size() == 0 && uploaded == 0) {
		length := min(chunkSize, info.Size()-uploaded)
		reader := io.NewSectionReader(file, uploaded, length)
		updated, err := hub.UploadTransferFile(
			ctx, localDeviceID, transfer.TransferID, 0, uploaded, info.Size(), reader,
		)
		if err != nil {
			return fmt.Errorf("upload paused at %d bytes; resume with --resume %s: %w", uploaded, transfer.TransferID, err)
		}
		next := updated.Files[0].Uploaded
		if next <= uploaded && info.Size() > 0 {
			return fmt.Errorf("upload made no progress at %d bytes", uploaded)
		}
		uploaded = next
		printTransferProgress(uploaded, info.Size(), uploaded-startedOffset, time.Since(startedAt))
		if info.Size() == 0 {
			break
		}
	}
	fmt.Printf("\nsent %s (%d bytes) as %s\n", sendName, info.Size(), transfer.TransferID)
	if wait {
		return waitForTransfer(ctx, transfer.TransferID)
	}
	return nil
}

func waitForTransfer(ctx context.Context, transferID string) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		transfers, err := hub.Transfers(ctx, localDeviceID, "sender", "")
		if err != nil {
			return err
		}
		for _, transfer := range transfers {
			if transfer.TransferID != transferID {
				continue
			}
			fmt.Fprintf(os.Stderr, "\rtransfer %s: %-12s", transferID, transfer.State)
			switch transfer.State {
			case "complete":
				fmt.Fprintln(os.Stderr)
				return nil
			case "declined", "canceled", "expired":
				fmt.Fprintln(os.Stderr)
				return fmt.Errorf("transfer %s", transfer.State)
			}
		}
		select {
		case <-ctx.Done():
			_, _ = hub.TransferAction(context.Background(), localDeviceID, transferID, "cancel")
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func printTransferProgress(uploaded, total, sessionBytes int64, elapsed time.Duration) {
	if elapsed <= 0 {
		elapsed = time.Millisecond
	}
	rate := float64(sessionBytes) / elapsed.Seconds()
	percent := 100.0
	if total > 0 {
		percent = float64(uploaded) * 100 / float64(total)
	}
	eta := time.Duration(0)
	if rate > 0 && uploaded < total {
		eta = time.Duration(float64(total-uploaded)/rate) * time.Second
	}
	fmt.Fprintf(os.Stderr, "\r%6.2f%%  %s/s  ETA %s", percent, humanBytes(int64(rate)), eta.Round(time.Second))
}

func humanBytes(value int64) string {
	const unit = int64(1024)
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := unit, 0
	for quotient := value / unit; quotient >= unit && exponent < 4; quotient /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGTPE"[exponent])
}

func archiveDirectory(source string) (string, func(), error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return "", func() {}, err
	}
	temp, err := os.CreateTemp("", filepath.Base(source)+"-*.tar")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.Remove(temp.Name()) }
	writer := tar.NewWriter(temp)
	walkErr := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == source {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to archive symlink %s", path)
		}
		relative, err := filepath.Rel(filepath.Dir(source), path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.ModTime = time.Unix(0, 0)
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeErr := writer.Close()
	fileCloseErr := temp.Close()
	if walkErr != nil {
		cleanup()
		return "", func() {}, walkErr
	}
	if closeErr != nil {
		cleanup()
		return "", func() {}, closeErr
	}
	if fileCloseErr != nil {
		cleanup()
		return "", func() {}, fileCloseErr
	}
	return temp.Name(), cleanup, nil
}

func cmdTransfers(ctx context.Context, args []string) error {
	transfers, err := hub.Transfers(ctx, localDeviceID, "", "")
	if err != nil {
		return err
	}
	if len(transfers) == 0 {
		fmt.Println("(no transfers)")
		return nil
	}
	for _, transfer := range transfers {
		direction, peer := "from", transfer.FromDevice
		if transfer.FromDevice == localDeviceID {
			direction, peer = "to", transfer.ToDevice
		}
		fmt.Printf("%s  %-12s %s %s  %d file(s)\n", transfer.TransferID, transfer.State, direction, peer, len(transfer.Files))
	}
	return nil
}

func cmdReceive(ctx context.Context, args []string) error {
	var id, destination string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--id":
			if i+1 < len(args) {
				id = args[i+1]
				i++
			}
		case "--to":
			if i+1 < len(args) {
				destination = args[i+1]
				i++
			}
		}
	}
	if destination == "" {
		home, _ := os.UserHomeDir()
		destination = filepath.Join(home, "Downloads")
	}
	if id == "" {
		pending, err := hub.Transfers(ctx, localDeviceID, "receiver", "offered")
		if err != nil {
			return err
		}
		if len(pending) != 1 {
			return fmt.Errorf("specify --id (found %d pending transfers)", len(pending))
		}
		id = pending[0].TransferID
	}
	transfers, err := hub.Transfers(ctx, localDeviceID, "receiver", "")
	if err != nil {
		return err
	}
	var selected *protocol.Transfer
	for i := range transfers {
		if transfers[i].TransferID == id {
			selected = &transfers[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("transfer %s not found", id)
	}
	if selected.State == "offered" {
		selected, err = hub.TransferAction(ctx, localDeviceID, id, "accept")
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for i, manifest := range selected.Files {
		finalPath := filepath.Join(destination, filepath.Base(manifest.Name))
		if _, err := os.Stat(finalPath); err == nil {
			return fmt.Errorf("refusing to overwrite %s", finalPath)
		}
		temp, err := os.CreateTemp(destination, ".tg-clipboard-*.part")
		if err != nil {
			return err
		}
		tempPath := temp.Name()
		var downloadErr error
		if selected.Mode == "direct" {
			downloadErr = hub.DownloadDirectTransferFile(ctx, *selected, i, temp)
		} else {
			downloadErr = hub.DownloadTransferFile(ctx, localDeviceID, id, i, temp)
		}
		if downloadErr != nil {
			temp.Close()
			os.Remove(tempPath)
			return downloadErr
		}
		if err := temp.Close(); err != nil {
			return err
		}
		if got, err := hashFile(tempPath); err != nil || !strings.EqualFold(got, manifest.SHA256) {
			os.Remove(tempPath)
			return fmt.Errorf("sha256 verification failed for %s", manifest.Name)
		}
		if err := os.Rename(tempPath, finalPath); err != nil {
			return err
		}
		fmt.Printf("received %s\n", finalPath)
	}
	_, err = hub.TransferAction(ctx, localDeviceID, id, "complete")
	return err
}

func resolveDevice(ctx context.Context, target string) (string, error) {
	device, err := resolveDeviceRecord(ctx, target)
	if err != nil {
		return "", err
	}
	return device.DeviceID, nil
}

func resolveDeviceRecord(ctx context.Context, target string) (protocol.Device, error) {
	devices, err := hub.Devices(ctx)
	if err != nil {
		return protocol.Device{}, err
	}
	var matches []protocol.Device
	for _, device := range devices {
		if device.DeviceID == target || strings.EqualFold(device.Name, target) {
			matches = append(matches, device)
		}
	}
	if len(matches) != 1 {
		return protocol.Device{}, fmt.Errorf("device %q matched %d devices", target, len(matches))
	}
	return matches[0], nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func defaultStateDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "tg-clipboard")
	}
	return filepath.Join(os.TempDir(), "tg-clipboard")
}

func cmdDevices(ctx context.Context) error {
	devices, err := hub.Devices(ctx)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		fmt.Println("(no registered devices)")
		return nil
	}
	for _, device := range devices {
		state := "offline"
		if device.Online {
			state = "online"
		}
		fmt.Printf("%-24s %-10s %-8s %s\n", device.Name, device.Platform, state, device.DeviceID)
	}
	return nil
}

func cmdGet(ctx context.Context, args []string) error {
	var outFile string
	for i, arg := range args {
		if (arg == "-o" || arg == "--output") && i+1 < len(args) {
			outFile = args[i+1]
			break
		}
	}

	if outFile != "" {
		blob, err := hub.Download(ctx, 0)
		if errors.Is(err, hubclient.ErrNoCurrentClip) {
			fmt.Fprintln(os.Stderr, "(clipboard empty)")
			return nil
		}
		if err != nil {
			return err
		}
		if err := os.WriteFile(outFile, blob.Data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "saved %s (%d bytes) to %s\n", blob.MimeType, len(blob.Data), outFile)
		return nil
	}

	item, err := hub.Current(ctx)
	if errors.Is(err, hubclient.ErrNoCurrentClip) {
		fmt.Fprintln(os.Stderr, "(clipboard empty)")
		return nil
	}
	if err != nil {
		return err
	}

	if item.IsText() {
		fmt.Print(item.Content)
		return nil
	}

	// Binary content.
	fmt.Fprintf(os.Stderr, "[%s, %d bytes] use -o <file> to save\n", item.MimeType, len(item.Data))
	return nil
}

func cmdPut(ctx context.Context, args []string) error {
	var (
		mimeType string
		filePath string
		textArgs []string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mime":
			if i+1 < len(args) {
				mimeType = args[i+1]
				i++
			}
		case "--file":
			if i+1 < len(args) {
				filePath = args[i+1]
				i++
			}
		default:
			textArgs = append(textArgs, args[i])
		}
	}

	var payload hubclient.PutRequest

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		if mimeType == "" {
			mimeType = mimeFromExt(filepath.Ext(filePath))
		}
		if strings.HasPrefix(mimeType, "text/") {
			payload = hubclient.PutRequest{Content: string(data), MimeType: mimeType}
		} else {
			payload = hubclient.PutRequest{Data: data, MimeType: mimeType}
		}
	} else {
		var content string
		if len(textArgs) > 0 {
			content = strings.Join(textArgs, " ")
		} else {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			content = string(data)
		}
		if content == "" {
			return fmt.Errorf("no content provided")
		}
		if mimeType == "" {
			mimeType = "text/plain"
		}
		payload = hubclient.PutRequest{Content: content, MimeType: mimeType}
	}

	item, err := hub.Put(ctx, payload)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "stored (seq=%d, %s, %d bytes)\n", item.Seq, item.MimeType, len(item.RawBytes()))
	return nil
}

func cmdHistory(ctx context.Context, args []string) error {
	limit := 20
	for i, arg := range args {
		if (arg == "-n" || arg == "--limit") && i+1 < len(args) {
			if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
				limit = n
			}
			break
		}
	}

	items, err := hub.History(ctx, limit)
	if err != nil {
		return err
	}

	for _, item := range items {
		age := time.Since(item.CreatedAt).Truncate(time.Second)
		if item.IsText() {
			preview := item.Content
			if len(preview) > 80 {
				preview = preview[:77] + "..."
			}
			preview = strings.ReplaceAll(preview, "\n", "\\n")
			fmt.Printf("#%-4d [%s ago] %s  %s  %q\n", item.Seq, age, item.Source, item.MimeType, preview)
		} else {
			fmt.Printf("#%-4d [%s ago] %s  %s  [%d bytes]\n", item.Seq, age, item.Source, item.MimeType, len(item.Data))
		}
	}
	return nil
}

func cmdClear(ctx context.Context, args []string) error {
	clearLocal := false
	for _, arg := range args {
		if arg == "--local" {
			clearLocal = true
		}
	}

	if err := hub.Clear(ctx); err != nil {
		return err
	}

	if clearLocal {
		localClipboard, err := clipboard.New()
		if err != nil {
			return err
		}
		if err := localClipboard.Clear(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "cleared hub clipboard/history and the local system clipboard")
		return nil
	}

	fmt.Fprintln(os.Stderr, "cleared hub clipboard/history")
	return nil
}

func cmdStatus(ctx context.Context) error {
	status, err := hub.Status(ctx)
	if err != nil {
		return err
	}

	keys := make([]string, 0, len(status))
	for key := range status {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		fmt.Printf("%-24s %v\n", key+":", status[key])
	}
	if devices, err := hub.Devices(ctx); err == nil {
		for _, device := range devices {
			if device.DeviceID != localDeviceID {
				continue
			}
			for _, capability := range device.Capabilities {
				if strings.HasPrefix(capability, "privacy-detector:") {
					fmt.Printf("%-24s %s\n", "privacy_detector:", strings.TrimPrefix(capability, "privacy-detector:"))
				}
			}
		}
	}
	return nil
}

func cmdPause() error {
	path := pauseFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte("paused\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "clipboard sync paused")
	return nil
}

func cmdResume() error {
	path := pauseFilePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Fprintln(os.Stderr, "clipboard sync resumed")
	return nil
}

func pauseFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tg-clipboard", "paused")
}

func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".html", ".htm":
		return "text/html"
	case ".txt":
		return "text/plain"
	case ".tar":
		return "application/x-tar"
	default:
		return "application/octet-stream"
	}
}
