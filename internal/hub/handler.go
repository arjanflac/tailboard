package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
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
	handle("GET /api/clip", "/api/clip", getClipHandler(h))
	handle("DELETE /api/clip", "/api/clip", clearClipHandler(h))
	handle("GET /api/clip/history", "/api/clip/history", historyHandler(h))
	handle("GET /api/clip/stream", "/api/clip/stream", streamHandler(h, obs))
	handle("GET /api/capabilities", "/api/capabilities", capabilitiesHandler(h))
	handle("POST /api/devices/register", "/api/devices/register", registerDeviceHandler(h))
	handle("GET /api/devices", "/api/devices", devicesHandler(h))
	handle("DELETE /api/devices/{id}", "/api/devices", removeDeviceHandler(h))
	handle("GET /api/status", "/api/status", statusHandler(h, obs))
	handle("GET /healthz", "/healthz", healthHandler(h, obs))
	handle("GET /readyz", "/readyz", readinessHandler(obs))
	handle("GET /metrics", "/metrics", metricsHandler(h, obs))
}

type postClipRequest struct {
	Content  string `json:"content,omitempty"`
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

		if req.Content == "" {
			writeAPIError(w, http.StatusBadRequest, "empty_content", "content cannot be empty", map[string]string{"field": "content"})
			return
		}

		source := identFn(r)
		item, isNew := h.Put(PutInput{
			Content:  req.Content,
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
				"payload_bytes", len(item.Content),
			)
		}
	}
}

func capabilitiesHandler(_ *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, protocol.Capabilities{
			HubVersion:      "dev",
			ProtocolVersion: protocol.ProtocolVersion,
			Features:        map[string]bool{"devices": true, "hub_role": true},
			Limits: protocol.CapabilityLimits{
				MaxClipSize: protocol.MaxContentSize,
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

func removeDeviceHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.RemoveDevice(r.PathValue("id")) {
			writeAPIError(w, http.StatusNotFound, "device_not_found", "no device with that id is registered", nil)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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

func streamHandler(h *Hub, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.URL.Query().Get("device_id")
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
		h.DeviceConnected(deviceID)
		defer h.DeviceDisconnected(deviceID)
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

		// Confirm that the HTTP upgrade completed before clients report a
		// healthy live connection. URLSessionWebSocketTask.resume() only starts
		// an attempt; it does not mean the server accepted the WebSocket.
		if err := wsjson.Write(ctx, conn, protocol.WSMessage{Type: "ready"}); err != nil {
			slog.Debug(
				"websocket ready write failed",
				"component", "hub_stream",
				"request_id", requestIDFromContext(r.Context()),
				"error", err,
			)
			return
		}
		// This protocol is server-push-only. CloseRead consumes control frames so
		// the hub observes iOS scene transitions and peer disconnects instead of
		// retaining zombie subscribers indefinitely.
		peerClosed := conn.CloseRead(ctx)

		// Clipboard sync is last-write-wins. On reconnect, send only the latest
		// state instead of serializing the full history.
		// Full history remains available through the explicit history APIs.
		var replayedUpTo uint64
		if s := r.URL.Query().Get("since_seq"); s != "" {
			if sinceSeq, err := strconv.ParseUint(s, 10, 64); err == nil {
				if item := h.Get(); item != nil && item.Seq > sinceSeq {
					msg := protocol.WSMessage{Type: "clip_update", Item: item}
					if err := wsjson.Write(ctx, conn, msg); err != nil {
						slog.Debug(
							"websocket catch-up write failed",
							"component", "hub_stream",
							"request_id", requestIDFromContext(r.Context()),
							"error", err,
						)
						return
					}
					replayedUpTo = item.Seq
					obs.RecordWSCatchup(1)
					slog.Info(
						"websocket catch-up replay",
						"component", "hub_stream",
						"request_id", requestIDFromContext(r.Context()),
						"since_sequence", sinceSeq,
						"replayed_items", 1,
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
			case <-peerClosed.Done():
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
