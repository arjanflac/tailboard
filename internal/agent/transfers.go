package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

func (a *Agent) handleTransferOffer(ctx context.Context, transfer protocol.Transfer) {
	switch a.transferPolicy {
	case "off":
		return
	case "ask":
		message := transferSummary(transfer)
		notify("Incoming ClipHub transfer", message+" — run tailclip receive --id "+transfer.TransferID)
		slog.Info("incoming transfer awaiting consent", "component", "clipd_transfers", "transfer_id", transfer.TransferID, "from_device", transfer.FromDevice)
		return
	case "accept":
		if len(a.transferAllow) == 0 {
			slog.Warn("auto-accept requires --transfer-allow", "component", "clipd_transfers", "transfer_id", transfer.TransferID)
			return
		}
		if _, allowed := a.transferAllow[transfer.FromDevice]; !allowed {
			slog.Info("incoming transfer is not allowlisted", "component", "clipd_transfers", "transfer_id", transfer.TransferID, "from_device", transfer.FromDevice)
			return
		}
	}

	accepted, err := a.client.TransferAction(ctx, a.deviceID, transfer.TransferID, "accept")
	if err != nil {
		slog.Error("failed to accept transfer", "component", "clipd_transfers", "transfer_id", transfer.TransferID, "error", err)
		return
	}
	if err := a.receiveTransfer(ctx, *accepted); err != nil {
		slog.Error("transfer download failed", "component", "clipd_transfers", "transfer_id", accepted.TransferID, "error", err)
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
		if err := a.downloadTransferFile(ctx, accepted.TransferID, index, manifest); err != nil {
			return fmt.Errorf("download %s: %w", manifest.Name, err)
		}
	}
	if _, err := a.client.TransferAction(ctx, a.deviceID, accepted.TransferID, "complete"); err != nil {
		return err
	}
	notify("ClipHub transfer complete", transferSummary(accepted)+" saved to "+a.downloadDir)
	slog.Info("transfer complete", "component", "clipd_transfers", "transfer_id", accepted.TransferID, "download_dir", a.downloadDir)
	return nil
}

func (a *Agent) downloadTransferFile(ctx context.Context, transferID string, index int, manifest protocol.TransferFile) error {
	name := filepath.Base(strings.ReplaceAll(manifest.Name, "\\", "/"))
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("unsafe filename %q", manifest.Name)
	}
	destination := filepath.Join(a.downloadDir, name)
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("refusing to overwrite %s", destination)
	}
	temp, err := os.CreateTemp(a.downloadDir, ".cliphub-*.part")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := a.client.DownloadTransferFile(ctx, a.deviceID, transferID, index, temp); err != nil {
		temp.Close()
		return err
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
	return os.Rename(tempPath, destination)
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
		slog.Debug("native notification failed", "component", "clipd_transfers", "error", err)
	}
}
