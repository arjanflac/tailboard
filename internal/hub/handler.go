package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

// IdentityFunc extracts a node/source name from an HTTP request.
type IdentityFunc func(r *http.Request) string

// Register mounts all API routes on mux.
func Register(mux *http.ServeMux, h *Hub, identFn IdentityFunc, observers ...*Observer) {
	obs := NewObserver()
	if len(observers) > 0 && observers[0] != nil {
		obs = observers[0]
	}

	handle := func(pattern string, route string, handler http.HandlerFunc) {
		mux.Handle(pattern, withObservability(obs, route, handler))
	}

	handle("POST /api/clip", "/api/clip", postClipHandler(h, identFn, obs))
	handle("POST /api/clip/blob", "/api/clip/blob", postBlobHandler(h, identFn, obs))
	handle("GET /api/clip", "/api/clip", getClipHandler(h))
	handle("GET /api/clip/blob", "/api/clip/blob", getBlobHandler(h))
	handle("DELETE /api/clip", "/api/clip", clearClipHandler(h))
	handle("GET /api/clip/history", "/api/clip/history", historyHandler(h))
	handle("GET /api/clip/history/page", "/api/clip/history/page", historyPageHandler(h))
	handle("GET /api/clip/stream", "/api/clip/stream", streamHandler(h, obs))
	handle("GET /api/capabilities", "/api/capabilities", capabilitiesHandler(h))
	handle("POST /api/devices/register", "/api/devices/register", registerDeviceHandler(h))
	handle("GET /api/devices", "/api/devices", devicesHandler(h))
	handle("POST /api/transfers", "/api/transfers", createTransferHandler(h))
	handle("GET /api/transfers", "/api/transfers", listTransfersHandler(h))
	handle("PUT /api/transfers/{id}/files/{index}", "/api/transfers/files/upload", uploadTransferFileHandler(h))
	handle("GET /api/transfers/{id}/files/{index}", "/api/transfers/files/download", downloadTransferFileHandler(h))
	handle("POST /api/transfers/{id}/{action}", "/api/transfers/action", transferActionHandler(h))
	handle("DELETE /api/transfers/{id}", "/api/transfers", cancelTransferHandler(h))
	handle("GET /api/status", "/api/status", statusHandler(h, obs))
	handle("GET /healthz", "/healthz", healthHandler(h, obs))
	handle("GET /readyz", "/readyz", readinessHandler(obs))
	handle("GET /metrics", "/metrics", metricsHandler(h, obs))
}

type postClipRequest struct {
	Content  string `json:"content,omitempty"`
	Data     []byte `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	DeviceID string `json:"device_id,omitempty"`
}

func postClipHandler(h *Hub, identFn IdentityFunc, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := readLimitedBody(r)
		if err != nil {
			writeBodyReadError(w, err)
			return
		}

		var req postClipRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
			return
		}

		// Default to text/plain for backward compatibility.
		if req.MimeType == "" {
			req.MimeType = "text/plain"
		}

		if strings.HasPrefix(req.MimeType, "text/") {
			if req.Content == "" {
				writeAPIError(w, http.StatusBadRequest, "empty_content", "content cannot be empty for text payloads", map[string]string{"field": "content"})
				return
			}
		} else if len(req.Data) == 0 {
			writeAPIError(w, http.StatusBadRequest, "empty_data", "data cannot be empty for binary payloads", map[string]string{"field": "data"})
			return
		}

		source := identFn(r)
		item, isNew := h.Put(PutInput{
			MimeType: req.MimeType,
			Content:  req.Content,
			Data:     req.Data,
			Source:   source,
			DeviceID: firstNonEmpty(req.DeviceID, r.Header.Get("X-Clip-Device-ID")),
		})

		if isNew {
			obs.RecordClipStored()
		} else {
			obs.RecordClipDeduplicated()
		}
		writeJSON(w, statusForNewItem(isNew), item)

		if isNew {
			slog.Info(
				"clip stored",
				"component", "hub_api",
				"request_id", requestIDFromContext(r.Context()),
				"sequence", item.Seq,
				"source", source,
				"mime_type", item.MimeType,
				"payload_bytes", len(item.RawBytes()),
			)
		}
	}
}

func postBlobHandler(h *Hub, identFn IdentityFunc, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := readLimitedBody(r)
		if err != nil {
			writeBodyReadError(w, err)
			return
		}
		if len(body) == 0 {
			writeAPIError(w, http.StatusBadRequest, "empty_data", "request body cannot be empty", nil)
			return
		}

		mimeType := normalizeMimeType(r.Header.Get("Content-Type"))
		input := PutInput{
			MimeType: mimeType,
			Source:   identFn(r),
			DeviceID: r.Header.Get("X-Clip-Device-ID"),
		}
		if strings.HasPrefix(mimeType, "text/") {
			input.Content = string(body)
		} else {
			input.Data = body
		}

		item, isNew := h.Put(input)
		if isNew {
			obs.RecordClipStored()
		} else {
			obs.RecordClipDeduplicated()
		}
		writeJSON(w, statusForNewItem(isNew), protocol.SummarizeClip(item))

		if isNew {
			slog.Info(
				"clip stored via blob endpoint",
				"component", "hub_api",
				"request_id", requestIDFromContext(r.Context()),
				"sequence", item.Seq,
				"source", input.Source,
				"mime_type", item.MimeType,
				"payload_bytes", len(item.RawBytes()),
			)
		}
	}
}

func capabilitiesHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, protocol.Capabilities{
			HubVersion:      "dev",
			ProtocolVersion: protocol.ProtocolVersion,
			Features:        map[string]bool{"transfers": true, "direct_fetch": true, "devices": true, "hub_role": true, "e2ee": false},
			Limits: protocol.CapabilityLimits{
				MaxClipSize:     protocol.MaxContentSize,
				MaxTransferSize: h.transfers.maxSize,
			},
		})
	}
}

func registerDeviceHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := readLimitedBody(r)
		if err != nil {
			writeBodyReadError(w, err)
			return
		}
		var req protocol.RegisterDeviceRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
			return
		}
		if req.DeviceID == "" || req.Name == "" || req.Platform == "" {
			writeAPIError(w, http.StatusBadRequest, "invalid_device", "device_id, name, and platform are required", nil)
			return
		}
		writeJSON(w, http.StatusOK, h.RegisterDevice(req))
	}
}

func devicesHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, h.Devices())
	}
}

func createTransferHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req protocol.CreateTransferRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", nil)
			return
		}
		transfer, err := h.transfers.create(deviceIdentity(r), req)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, ErrSpoolQuota) {
				status = http.StatusInsufficientStorage
			}
			writeAPIError(w, status, "transfer_create_failed", err.Error(), nil)
			return
		}
		h.publishTransfer(transfer)
		urls := make([]string, len(transfer.Files))
		for i := range urls {
			urls[i] = fmt.Sprintf("/api/transfers/%s/files/%d", transfer.TransferID, i)
		}
		writeJSON(w, http.StatusCreated, protocol.CreateTransferResponse{Transfer: transfer, UploadURLs: urls})
	}
}

func listTransfersHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, h.transfers.list(deviceIdentity(r), r.URL.Query().Get("role"), r.URL.Query().Get("state")))
	}
}

func uploadTransferFileHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || index < 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_file_index", "file index must be a non-negative integer", nil)
			return
		}
		start := int64(0)
		if contentRange := r.Header.Get("Content-Range"); contentRange != "" {
			if _, err := fmt.Sscanf(contentRange, "bytes %d-", &start); err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_content_range", "Content-Range must start with bytes START-", nil)
				return
			}
		}
		transfer, err := h.transfers.get(r.PathValue("id"))
		if err != nil {
			writeTransferError(w, err)
			return
		}
		if transfer.FromDevice != deviceIdentity(r) {
			writeAPIError(w, http.StatusForbidden, "transfer_forbidden", "only the sender may upload", nil)
			return
		}
		transfer, err = h.transfers.upload(transfer.TransferID, index, start, r.Body)
		if err != nil {
			writeTransferError(w, err)
			return
		}
		h.publishTransfer(transfer)
		writeJSON(w, http.StatusOK, transfer)
	}
}

func downloadTransferFileHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || index < 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_file_index", "file index must be a non-negative integer", nil)
			return
		}
		transfer, err := h.transfers.get(r.PathValue("id"))
		if err != nil {
			writeTransferError(w, err)
			return
		}
		if transfer.ToDevice != deviceIdentity(r) {
			writeAPIError(w, http.StatusForbidden, "transfer_forbidden", "only the receiver may download", nil)
			return
		}
		file, manifest, err := h.transfers.openFile(transfer.TransferID, index)
		if err != nil {
			writeTransferError(w, err)
			return
		}
		defer file.Close()
		if current, err := h.transfers.get(transfer.TransferID); err == nil {
			h.publishTransfer(current)
		}
		w.Header().Set("Content-Type", manifest.MIME)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": manifest.Name}))
		http.ServeContent(w, r, manifest.Name, transfer.CreatedAt, file)
	}
}

func transferActionHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action := r.PathValue("action")
		if action != "accept" && action != "decline" && action != "complete" {
			writeAPIError(w, http.StatusNotFound, "unknown_transfer_action", "unknown transfer action", nil)
			return
		}
		transfer, err := h.transfers.transition(r.PathValue("id"), deviceIdentity(r), action)
		if err != nil {
			writeTransferError(w, err)
			return
		}
		h.publishTransfer(transfer)
		writeJSON(w, http.StatusOK, transfer)
	}
}

func cancelTransferHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		transfer, err := h.transfers.cancel(r.PathValue("id"), deviceIdentity(r))
		if err != nil {
			writeTransferError(w, err)
			return
		}
		h.publishTransfer(transfer)
		writeJSON(w, http.StatusOK, transfer)
	}
}

func deviceIdentity(r *http.Request) string {
	return firstNonEmpty(r.Header.Get("X-Clip-Device-ID"), r.URL.Query().Get("device_id"))
}

func writeTransferError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTransferNotFound):
		writeAPIError(w, http.StatusNotFound, "transfer_not_found", err.Error(), nil)
	case errors.Is(err, ErrTransferConflict):
		writeAPIError(w, http.StatusConflict, "transfer_conflict", err.Error(), nil)
	default:
		writeAPIError(w, http.StatusBadRequest, "transfer_failed", err.Error(), nil)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func getClipHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item := h.Get()
		if item == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func getBlobHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seq, ok := parseOptionalUintQuery(w, r, "seq")
		if !ok {
			return
		}

		var (
			item *protocol.ClipItem
			err  error
		)
		if seq == 0 {
			item = h.Get()
			if item == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		} else {
			item, err = h.GetBySeq(seq)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "history_lookup_failed", "failed to load clip from history", nil)
				return
			}
			if item == nil {
				writeAPIError(w, http.StatusNotFound, "clip_not_found", "no clip exists for the requested sequence", map[string]string{"seq": strconv.FormatUint(seq, 10)})
				return
			}
		}

		w.Header().Set("Content-Type", item.MimeType)
		w.Header().Set("Content-Length", strconv.Itoa(len(item.RawBytes())))
		w.Header().Set("X-Clip-Seq", strconv.FormatUint(item.Seq, 10))
		w.Header().Set("X-Clip-Hash", item.Hash)
		w.Header().Set("X-Clip-Source", item.Source)
		if _, err := w.Write(item.RawBytes()); err != nil {
			slog.Debug("blob write failed", "err", err)
		}
	}
}

func clearClipHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.Clear(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "clear_failed", "failed to clear current clip and history", nil)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func historyHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, ok := parseLimitQuery(w, r, 50, protocol.MaxHistoryPageLimit)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, h.History(limit))
	}
}

func historyPageHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, ok := parseLimitQuery(w, r, protocol.DefaultHistoryLimit, protocol.MaxHistoryPageLimit)
		if !ok {
			return
		}
		cursor, ok := parseOptionalUintQuery(w, r, "cursor")
		if !ok {
			return
		}

		items, nextCursor, hasMore, err := h.HistoryPage(limit, cursor)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "history_page_failed", "failed to load paged history", nil)
			return
		}

		page := protocol.HistoryPage{
			Items:   make([]protocol.ClipSummary, 0, len(items)),
			HasMore: hasMore,
		}
		for _, item := range items {
			page.Items = append(page.Items, protocol.SummarizeClip(item))
		}
		page.NextCursor = nextCursor
		writeJSON(w, http.StatusOK, page)
	}
}

func streamHandler(h *Hub, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.URL.Query().Get("device_id")
		h.SetDeviceOnline(deviceID, true)
		defer h.SetDeviceOnline(deviceID, false)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			slog.Error(
				"websocket accept failed",
				"component", "hub_stream",
				"request_id", requestIDFromContext(r.Context()),
				"error", err,
			)
			return
		}
		defer conn.CloseNow()
		obs.RecordWSConnect()
		defer obs.RecordWSDisconnect()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go func() {
			select {
			case <-ctx.Done():
			case <-obs.ShutdownContext().Done():
				cancel()
			}
		}()

		// Subscribe first so we don't miss items arriving during catch-up.
		sub := h.Subscribe(ctx)
		defer sub.cancel()

		// Catch-up replay: send items missed since the given seq.
		var replayedUpTo uint64
		if s := r.URL.Query().Get("since_seq"); s != "" {
			if sinceSeq, err := strconv.ParseUint(s, 10, 64); err == nil {
				missed := h.Since(sinceSeq)
				for _, item := range missed {
					msg := protocol.WSMessage{Type: "clip_update", Item: &item}
					if err := wsjson.Write(ctx, conn, msg); err != nil {
						slog.Debug(
							"websocket catch-up write failed",
							"component", "hub_stream",
							"request_id", requestIDFromContext(r.Context()),
							"error", err,
						)
						return
					}
					if item.Seq > replayedUpTo {
						replayedUpTo = item.Seq
					}
				}
				if len(missed) > 0 {
					obs.RecordWSCatchup(len(missed))
					slog.Info(
						"websocket catch-up replay",
						"component", "hub_stream",
						"request_id", requestIDFromContext(r.Context()),
						"since_sequence", sinceSeq,
						"replayed_items", len(missed),
					)
				}
			}
		}

		slog.Info(
			"subscriber connected",
			"component", "hub_stream",
			"request_id", requestIDFromContext(r.Context()),
			"remote_addr", r.RemoteAddr,
		)

		for {
			select {
			case <-ctx.Done():
				_ = conn.Close(websocket.StatusNormalClosure, "bye")
				return
			case item := <-sub.C:
				if item.Seq <= replayedUpTo {
					continue
				}
				msg := protocol.WSMessage{Type: "clip_update", Item: &item}
				if err := wsjson.Write(ctx, conn, msg); err != nil {
					slog.Debug(
						"websocket write failed",
						"component", "hub_stream",
						"request_id", requestIDFromContext(r.Context()),
						"error", err,
					)
					return
				}
			case transfer := <-sub.Transfers:
				if deviceID == "" || (transfer.FromDevice != deviceID && transfer.ToDevice != deviceID) {
					continue
				}
				msg := protocol.WSMessage{Type: "transfer_state", Transfer: &transfer}
				if transfer.State == "offered" {
					msg.Type = "transfer_offer"
				}
				if err := wsjson.Write(ctx, conn, msg); err != nil {
					return
				}
			}
		}
	}
}

func statusHandler(h *Hub, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, obs.Status(h))
	}
}

func healthHandler(h *Hub, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"ready":  obs.Ready(),
			"uptime": time.Since(h.StartedAt()).String(),
		})
	}
}

func readinessHandler(obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statusCode := http.StatusOK
		status := "ready"
		if !obs.Ready() {
			statusCode = http.StatusServiceUnavailable
			status = "shutting_down"
		}
		writeJSON(w, statusCode, map[string]any{
			"status": status,
			"ready":  obs.Ready(),
		})
	}
}

func metricsHandler(h *Hub, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, obs.Metrics(h))
	}
}

func readLimitedBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, int64(protocol.MaxContentSize)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > protocol.MaxContentSize {
		return nil, errRequestTooLarge
	}
	return body, nil
}

func normalizeMimeType(headerValue string) string {
	if headerValue == "" {
		return "application/octet-stream"
	}
	mimeType, _, err := mime.ParseMediaType(headerValue)
	if err != nil || mimeType == "" {
		return "application/octet-stream"
	}
	return mimeType
}

func parseLimitQuery(w http.ResponseWriter, r *http.Request, fallback int, max int) (int, bool) {
	limit := fallback
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer", map[string]string{"limit": raw})
			return 0, false
		}
		limit = n
	}
	if max > 0 && limit > max {
		limit = max
	}
	return limit, true
}

func parseOptionalUintQuery(w http.ResponseWriter, r *http.Request, key string) (uint64, bool) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		writeAPIError(w, http.StatusBadRequest, "invalid_"+key, key+" must be a positive integer", map[string]string{key: raw})
		return 0, false
	}
	return value, true
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("encode response failed", "status", statusCode, "err", err)
	}
}

func writeAPIError(w http.ResponseWriter, status int, code string, message string, details map[string]string) {
	writeJSON(w, status, protocol.ErrorResponse{
		Error: protocol.APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func writeBodyReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, errRequestTooLarge) {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "content_too_large", "request body exceeds the maximum clip size", map[string]string{"max_bytes": strconv.Itoa(protocol.MaxContentSize)})
		return
	}
	writeAPIError(w, http.StatusBadRequest, "read_error", "failed to read request body", nil)
}

func statusForNewItem(isNew bool) int {
	if isNew {
		return http.StatusCreated
	}
	return http.StatusOK
}

var errRequestTooLarge = errors.New("request body exceeds max clip size")
