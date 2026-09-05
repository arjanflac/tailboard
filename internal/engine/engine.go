package engine

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/arjanflac/tailboard/internal/clipboard"
	"github.com/arjanflac/tailboard/internal/relay"
)

// Engine connects the Mac pasteboard directly to the in-memory network relay.
type Engine struct {
	board       clipboard.Clipboard
	relay       *relay.Relay
	name        string
	mu          sync.Mutex
	lastWritten string
}

func New(board clipboard.Clipboard, name string) *Engine {
	engine := &Engine{board: board, name: name}
	engine.relay = relay.New(engine.ApplyRemote, engine.ClearRemote)
	return engine
}

func (e *Engine) Relay() *relay.Relay { return e.relay }

func (e *Engine) Poll() {
	changed, err := e.board.Changed()
	if err != nil || !changed {
		return
	}
	text, err := e.board.ReadText()
	if errors.Is(err, clipboard.ErrNoText) {
		return
	}
	if err != nil {
		slog.Warn("could not read clipboard", "error", err)
		return
	}
	if text == "" || len(text) > relay.MaxTextBytes {
		return
	}

	e.mu.Lock()
	if text == e.lastWritten {
		e.lastWritten = ""
		e.mu.Unlock()
		return
	}
	e.lastWritten = ""
	e.mu.Unlock()
	e.relay.PutLocal(text, e.name)
}

func (e *Engine) ApplyRemote(text string) error {
	e.mu.Lock()
	e.lastWritten = text
	e.mu.Unlock()
	if err := e.board.WriteText(text); err != nil {
		e.mu.Lock()
		e.lastWritten = ""
		e.mu.Unlock()
		return err
	}
	return nil
}

func (e *Engine) ClearRemote() error {
	e.mu.Lock()
	e.lastWritten = ""
	e.mu.Unlock()
	return e.board.Clear()
}
