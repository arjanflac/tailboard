package engine

import "testing"

type fakeClipboard struct {
	text    string
	changed bool
	writes  []string
	clears  int
}

func (f *fakeClipboard) ReadText() (string, error) { return f.text, nil }
func (f *fakeClipboard) WriteText(text string) error {
	f.text = text
	f.changed = true
	f.writes = append(f.writes, text)
	return nil
}
func (f *fakeClipboard) Clear() error { f.text = ""; f.changed = true; f.clears++; return nil }
func (f *fakeClipboard) Changed() (bool, error) {
	changed := f.changed
	f.changed = false
	return changed, nil
}

func TestPollPublishesOnlyNewLocalText(t *testing.T) {
	board := &fakeClipboard{text: "hello", changed: true}
	engine := New(board, "Mac")
	engine.Poll()
	if current := engine.Relay().Current(); current == nil || current.Content != "hello" {
		t.Fatalf("current = %#v", current)
	}
	engine.Poll()
	if current := engine.Relay().Current(); current.Seq != 1 {
		t.Fatalf("sequence = %d, want 1", current.Seq)
	}
}

func TestRemoteWriteDoesNotEcho(t *testing.T) {
	board := &fakeClipboard{}
	engine := New(board, "Mac")
	if err := engine.ApplyRemote("from phone"); err != nil {
		t.Fatal(err)
	}
	engine.Poll()
	if current := engine.Relay().Current(); current != nil {
		t.Fatalf("remote write echoed as local: %#v", current)
	}
}

func TestClearRemoteClearsSystemClipboard(t *testing.T) {
	board := &fakeClipboard{text: "secret"}
	engine := New(board, "Mac")
	if err := engine.ClearRemote(); err != nil {
		t.Fatal(err)
	}
	if board.text != "" || board.clears != 1 {
		t.Fatalf("board = %#v", board)
	}
}
