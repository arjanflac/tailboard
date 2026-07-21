package agent

import (
	"context"
	"fmt"
	"log/slog"
	urlpkg "net/url"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/thalysguimaraes/tg-clipboard/internal/protocol"
)

// wsReadLimit is the WebSocket read limit: MaxContentSize + headroom for JSON framing.
const wsReadLimit = protocol.MaxContentSize + 64*1024

// WSClient connects to the hub's WebSocket stream and delivers updates.
type WSClient struct {
	URL         string
	OnUpdate    func(protocol.ClipItem)
	OnTransfer  func(protocol.Transfer)
	OnConnected func() // Called after each successful connect.
	lastSeq     uint64 // Tracks last seq received for reconnect catch-up.
}

// Run connects to the hub and reads updates until ctx is cancelled.
// It reconnects automatically with exponential backoff.
func (c *WSClient) Run(ctx context.Context) {
	backoff := time.Second

	for {
		if err := c.connect(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("websocket disconnected", "component", "tg-clipd_stream", "error", err, "retry_delay", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 60*time.Second)
			continue
		}
		backoff = time.Second
	}
}

func (c *WSClient) connect(ctx context.Context) error {
	url := c.URL
	if c.lastSeq > 0 {
		parsed, err := urlpkg.Parse(url)
		if err != nil {
			return err
		}
		query := parsed.Query()
		query.Set("since_seq", fmt.Sprintf("%d", c.lastSeq))
		parsed.RawQuery = query.Encode()
		url = parsed.String()
	}

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	conn.SetReadLimit(wsReadLimit)

	slog.Info("connected to hub", "component", "tg-clipd_stream", "hub_stream_url", url)

	if c.OnConnected != nil {
		c.OnConnected()
	}

	for {
		var msg protocol.WSMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return err
		}
		if msg.Type == "clip_update" && msg.Item != nil {
			c.lastSeq = msg.Item.Seq
			c.OnUpdate(*msg.Item)
		}
		if (msg.Type == "transfer_offer" || msg.Type == "transfer_state") && msg.Transfer != nil && c.OnTransfer != nil {
			c.OnTransfer(*msg.Transfer)
		}
	}
}
