package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/arjanflac/tailboard/internal/protocol"
)

func (a *Agent) handleTransferOffer(ctx context.Context, transfer protocol.Transfer) {
	switch a.transferPolicy {
	case "off":
		return
	case "ask":
		message := transferSummary(transfer)
		notify("Incoming tg-clipboard transfer", message+" — run tg-clip receive --id "+transfer.TransferID)
		slog.Info("incoming transfer awaiting consent", "component", "tg-clipd_transfers", "transfer_id", transfer.TransferID, "from_device", transfer.FromDevice)
		return
	case "accept":
		if len(a.transferAllow) == 0 {
			slog.Warn("auto-accept requires --transfer-allow", "component", "tg-clipd_transfers", "transfer_id", transfer.TransferID)
			return
		}
		if _, allowed := a.transferAllow[transfer.FromDevice]; !allowed {
			slog.Info("incoming transfer is not allowlisted", "component", "tg-clipd_transfers", "transfer_id", transfer.TransferID, "from_device", transfer.FromDevice)
			return
		}
	}

	a.receiveTransferOnce(ctx, transfer)
}

// recoverIncomingTransfers makes foreground/background delivery resilient to
// an engine restart or a transient WebSocket disconnect. Offered transfers are
// re-evaluated against the current policy; already-accepted transfers resume
// immediately because the receiver has already consented to them.
func (a *Agent) recoverIncomingTransfers(ctx context.Context) {
	transfers, err := a.client.Transfers(ctx, a.deviceID, "receiver", "")
	if err != nil {
		slog.Warn("incoming transfer recovery failed", "component", "tg-clipd_transfers", "error", err)
		return
	}
	slog.Info("recovering incoming transfers", "component", "tg-clipd_transfers", "count", len(transfers))
	for _, transfer := range transfers {
		if transfer.ToDevice != a.deviceID {
			continue
		}
		switch transfer.State {
		case "offered":
			go a.handleTransferOffer(ctx, transfer)
		case "accepted", "transferring":
			go a.receiveTransferOnce(ctx, transfer)
		}
	}
}

// receiveTransferOnce deduplicates live-stream and reconnect recovery events.
// A failed accepted transfer is canceled so the UI never lies indefinitely
// with a permanent "Receiving…" row.
func (a *Agent) receiveTransferOnce(ctx context.Context, transfer protocol.Transfer) {
	a.transferMu.Lock()
	if _, active := a.receiving[transfer.TransferID]; active {
		a.transferMu.Unlock()
		return
	}
	a.receiving[transfer.TransferID] = struct{}{}
	a.transferMu.Unlock()
	defer func() {
		a.transferMu.Lock()
		delete(a.receiving, transfer.TransferID)
		a.transferMu.Unlock()
	}()
	slog.Info("receiving transfer", "component", "tg-clipd_transfers", "transfer_id", transfer.TransferID, "state", transfer.State)
	a.downloadMu.Lock()
	defer a.downloadMu.Unlock()

	if err := a.receiveTransfer(ctx, transfer); err != nil {
		slog.Error("transfer download failed", "component", "tg-clipd_transfers", "transfer_id", transfer.TransferID, "error", err)
		if _, cancelErr := a.client.TransferAction(ctx, a.deviceID, transfer.TransferID, "cancel"); cancelErr != nil {
			slog.Error("failed to close broken transfer", "component", "tg-clipd_transfers", "transfer_id", transfer.TransferID, "error", cancelErr)
		}
	}
}

func (a *Agent) receiveTransfer(ctx context.Context, transfer protocol.Transfer) error {
	accepted := transfer
	if accepted.State == "offered" {
		updated, actionErr := a.client.TransferAction(ctx, a.deviceID, accepted.TransferID, "accept")
		if actionErr != nil {
			return actionErr
		}
		accepted = *updated
	}
	if err := os.MkdirAll(a.downloadDir, 0o755); err != nil {
		return err
	}
	for index, manifest := range accepted.Files {
		if err := a.downloadTransferFile(ctx, accepted, index, manifest); err != nil {
			return fmt.Errorf("download %s: %w", manifest.Name, err)
		}
	}
	if _, err := a.client.TransferAction(ctx, a.deviceID, accepted.TransferID, "complete"); err != nil {
		return err
	}
	notify("Tailboard transfer complete", transferSummary(accepted)+" saved to "+a.downloadDir)
	slog.Info("transfer complete", "component", "tg-clipd_transfers", "transfer_id", accepted.TransferID, "download_dir", a.downloadDir)
	return nil
}

func (a *Agent) downloadTransferFile(ctx context.Context, transfer protocol.Transfer, index int, manifest protocol.TransferFile) error {
	name := filepath.Base(strings.ReplaceAll(manifest.Name, "\\", "/"))
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("unsafe filename %q", manifest.Name)
	}
	if transfer.FromDevice != "" {
		name = sourceAwareFileName(name, a.transferSourceLabel(ctx, transfer.FromDevice))
	}
	slog.Info("preparing transfer file", "component", "tg-clipd_transfers", "transfer_id", transfer.TransferID, "file", name, "size", manifest.Size)
	alreadyPresent, err := transferFileAlreadyPresent(a.downloadDir, manifest)
	if err != nil {
		return err
	}
	if alreadyPresent {
		slog.Info("skipping duplicate transfer payload", "component", "tg-clipd_transfers", "transfer_id", transfer.TransferID, "file", name)
		return nil
	}
	temp, err := os.CreateTemp(a.downloadDir, ".tailboard-*.part")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	var downloadErr error
	if transfer.Mode == "direct" {
		downloadErr = a.client.DownloadDirectTransferFile(ctx, transfer, index, temp)
	} else {
		downloadErr = a.client.DownloadTransferFile(ctx, a.deviceID, transfer.TransferID, index, temp)
	}
	if downloadErr != nil {
		temp.Close()
		return downloadErr
	}
	if err := temp.Close(); err != nil {
		return err
	}
	hash, err := transferFileSHA256(tempPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(hash, manifest.SHA256) {
		return fmt.Errorf("sha256 mismatch")
	}
	return commitTransferFile(tempPath, a.downloadDir, name)
}

// transferFileAlreadyPresent avoids saving duplicate retries under a new name.
// It compares only same-sized regular files before hashing, keeping the common
// path cheap while making repeated iOS "Shared Image.png" sends idempotent.
func transferFileAlreadyPresent(directory string, manifest protocol.TransferFile) (bool, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() != manifest.Size {
			continue
		}
		hash, err := transferFileSHA256(filepath.Join(directory, entry.Name()))
		if err != nil {
			continue
		}
		if strings.EqualFold(hash, manifest.SHA256) {
			return true, nil
		}
	}
	return false, nil
}

// commitTransferFile never overwrites an existing download. Filename
// collisions become "Name (2).ext", "Name (3).ext", and so on. A hard link
// makes the final placement atomic on the same Downloads filesystem.
func commitTransferFile(tempPath, directory, name string) error {
	extension := filepath.Ext(name)
	stem := strings.TrimSuffix(name, extension)
	numberedStem := strings.TrimSuffix(stem, " 1")
	hasSourceNumber := numberedStem != stem
	for attempt := 0; attempt < 10_000; attempt++ {
		candidate := name
		if attempt > 0 {
			if hasSourceNumber {
				candidate = fmt.Sprintf("%s %d%s", numberedStem, attempt+1, extension)
			} else {
				candidate = fmt.Sprintf("%s (%d)%s", stem, attempt+1, extension)
			}
		}
		destination := filepath.Join(directory, candidate)
		if err := os.Link(tempPath, destination); err == nil {
			return nil
		} else if errors.Is(err, os.ErrExist) {
			continue
		} else {
			return err
		}
	}
	return fmt.Errorf("too many files named %q", name)
}

func (a *Agent) transferSourceLabel(ctx context.Context, deviceID string) string {
	devices, err := a.client.Devices(ctx)
	if err != nil {
		return ""
	}
	for _, device := range devices {
		if device.DeviceID != deviceID {
			continue
		}
		switch strings.ToLower(device.Platform) {
		case "ios":
			return "iPhone"
		case "android":
			if strings.Contains(strings.ToLower(device.Name), "pixel") {
				return "Pixel"
			}
			return "Android"
		default:
			return strings.TrimSpace(device.Name)
		}
	}
	return ""
}

// sourceAwareFileName gives generic mobile share names a stable identity while
// preserving meaningful originals such as camera filenames and documents.
func sourceAwareFileName(name, source string) string {
	if source == "" {
		return name
	}
	extension := filepath.Ext(name)
	switch strings.ToLower(extension) {
	case ".png", ".jpg", ".jpeg", ".heic", ".webp", ".gif":
	default:
		return name
	}
	stem := strings.TrimSuffix(name, extension)
	lowerStem := strings.ToLower(strings.TrimSpace(stem))
	if lowerStem != "shared image" && !strings.HasPrefix(lowerStem, "share_") {
		return name
	}
	return "Shared Image " + source + " 1" + extension
}

func transferFileSHA256(path string) (string, error) {
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

func transferSummary(transfer protocol.Transfer) string {
	var total int64
	for _, file := range transfer.Files {
		total += file.Size
	}
	return fmt.Sprintf("%d file(s), %d bytes from %s", len(transfer.Files), total, transfer.FromDevice)
}

func notify(title, message string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf("display notification %q with title %q", message, title)
		command = exec.Command("osascript", "-e", script)
	case "linux":
		command = exec.Command("notify-send", title, message)
	case "windows":
		command = exec.Command("powershell", "-NoProfile", "-Command", "Write-Output", title+": "+message)
	default:
		return
	}
	if err := command.Run(); err != nil {
		slog.Debug("native notification failed", "component", "tg-clipd_transfers", "error", err)
	}
}
