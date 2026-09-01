package hub

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
)

func newTestHub() *Hub {
	h := &Hub{
		subs:              make(map[*Subscriber]struct{}),
		devices:           make(map[string]protocol.Device),
		deviceConnections: make(map[string]int),
		startedAt:         time.Now(),
		stop:              make(chan struct{}),
	}
	h.publishCond = sync.NewCond(&h.publishMu)
	return h
}

func textInput(content, source string) PutInput {
	return PutInput{Content: content, Source: source}
}

type blockingStore struct {
	saveStarted chan protocol.ClipItem
	releaseSave chan struct{}
}

func newBlockingStore() *blockingStore {
	return &blockingStore{
		saveStarted: make(chan protocol.ClipItem, 8),
		releaseSave: make(chan struct{}, 8),
	}
}

func (s *blockingStore) Close() error { return nil }

func (s *blockingStore) LoadState() (uint64, *protocol.ClipItem, error) {
	return 0, nil, nil
}

func (s *blockingStore) ReplaceCurrent(item protocol.ClipItem) error {
	s.saveStarted <- item
	<-s.releaseSave
	return nil
}

func (s *blockingStore) DeleteAll() error {
	return nil
}

func (s *blockingStore) LoadDevices() ([]protocol.Device, error) {
	return nil, nil
}

func (s *blockingStore) SaveDevice(protocol.Device) error {
	return nil
}

func (s *blockingStore) DeleteDevice(string) error {
	return nil
}

func (s *blockingStore) allowOneSave() {
	s.releaseSave <- struct{}{}
}

func TestPutAndGet(t *testing.T) {
	h := newTestHub()

	item, isNew := h.Put(textInput("hello", "node1"))
	if !isNew {
		t.Fatal("first put should be new")
	}
	if item.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", item.Seq)
	}
	if item.Content != "hello" {
		t.Fatal("content mismatch")
	}

	got := h.Get()
	if got == nil || got.Seq != 1 {
		t.Fatal("Get should return current item")
	}
}

func TestMergeStringsKeepsCapabilitiesWithoutDuplicates(t *testing.T) {
	got := mergeStrings(
		[]string{"clipboard", "privacy-detector:x11-ewmh"},
		[]string{"clipboard", "privacy-detector:x11-ewmh"},
	)
	want := []string{"clipboard", "privacy-detector:x11-ewmh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDedup(t *testing.T) {
	h := newTestHub()

	h.Put(textInput("hello", "node1"))
	_, isNew := h.Put(textInput("hello", "node2"))
	if isNew {
		t.Fatal("duplicate content should not be new")
	}
	if h.Seq() != 1 {
		t.Fatal("seq should not increment on dedup")
	}
}

func TestMonotonicSeq(t *testing.T) {
	h := newTestHub()

	for i := 0; i < 10; i++ {
		item, _ := h.Put(textInput(string(rune('a'+i)), "node1"))
		if item.Seq != uint64(i+1) {
			t.Fatalf("expected seq %d, got %d", i+1, item.Seq)
		}
	}
}

func TestClearRemovesCurrent(t *testing.T) {
	h := newTestHub()
	h.Put(textInput("one", "node1"))
	h.Put(textInput("two", "node2"))

	if err := h.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	if got := h.Get(); got != nil {
		t.Fatalf("expected cleared current clip, got %+v", got)
	}
	if h.Seq() != 2 {
		t.Fatalf("expected clear to preserve seq 2, got %d", h.Seq())
	}
}

func TestSubscriberReceivesUpdates(t *testing.T) {
	h := newTestHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := h.Subscribe(ctx)

	h.Put(textInput("hello", "node1"))

	select {
	case item := <-sub.C:
		if item.Content != "hello" {
			t.Fatal("wrong content")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for subscriber update")
	}
}

func TestSubscriberCountAfterCancel(t *testing.T) {
	h := newTestHub()
	ctx, cancel := context.WithCancel(context.Background())

	h.Subscribe(ctx)
	if h.SubscriberCount() != 1 {
		t.Fatal("expected 1 subscriber")
	}

	cancel()
	time.Sleep(50 * time.Millisecond)

	if h.SubscriberCount() != 0 {
		t.Fatal("expected 0 subscribers after cancel")
	}
}

func TestPutReleasesStateLockBeforePersistence(t *testing.T) {
	h := newTestHub()
	store := newBlockingStore()
	h.store = store

	putDone := make(chan struct{})
	go func() {
		h.Put(textInput("hello", "node1"))
		close(putDone)
	}()

	saved := <-store.saveStarted
	if saved.Seq != 1 {
		t.Fatalf("expected first persisted seq to be 1, got %d", saved.Seq)
	}

	getDone := make(chan *protocol.ClipItem, 1)
	go func() {
		getDone <- h.Get()
	}()

	select {
	case item := <-getDone:
		if item == nil || item.Content != "hello" {
			t.Fatalf("expected Get to observe the in-memory update, got %+v", item)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Get blocked while persistence was in progress")
	}

	store.allowOneSave()

	select {
	case <-putDone:
	case <-time.After(time.Second):
		t.Fatal("Put did not finish after persistence was released")
	}
}

func TestConcurrentPutsPreserveSubscriberOrder(t *testing.T) {
	h := newTestHub()
	store := newBlockingStore()
	h.store = store

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx)

	done := make(chan struct{}, 2)
	go func() {
		h.Put(textInput("first", "node1"))
		done <- struct{}{}
	}()

	firstSaved := <-store.saveStarted
	if firstSaved.Seq != 1 {
		t.Fatalf("expected first persisted seq to be 1, got %d", firstSaved.Seq)
	}

	select {
	case item := <-sub.C:
		if item.Seq != 1 || item.Content != "first" {
			t.Fatalf("expected first subscriber item to be seq 1, got %+v", item)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first subscriber item")
	}

	go func() {
		h.Put(textInput("second", "node2"))
		done <- struct{}{}
	}()

	store.allowOneSave()

	secondSaved := <-store.saveStarted
	if secondSaved.Seq != 2 {
		t.Fatalf("expected second persisted seq to be 2, got %d", secondSaved.Seq)
	}

	select {
	case item := <-sub.C:
		if item.Seq != 2 || item.Content != "second" {
			t.Fatalf("expected second subscriber item to be seq 2, got %+v", item)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for second subscriber item")
	}

	store.allowOneSave()

	for range 2 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for puts to finish")
		}
	}
}

func TestSlowSubscribersReceiveLatestUpdate(t *testing.T) {
	h := newTestHub()
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	sub1 := h.Subscribe(ctx1)
	sub2 := h.Subscribe(ctx2)

	h.Put(textInput("first", "node1"))
	h.Put(textInput("second", "node2"))

	readLatest := func(t *testing.T, sub *Subscriber) uint64 {
		t.Helper()
		select {
		case item := <-sub.C:
			return item.Seq
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for subscriber update")
		}
		return 0
	}

	if seq := readLatest(t, sub1); seq != 2 {
		t.Fatalf("subscriber 1 saw stale seq %d", seq)
	}
	if seq := readLatest(t, sub2); seq != 2 {
		t.Fatalf("subscriber 2 saw stale seq %d", seq)
	}
}

func TestCurrentDoesNotExpireOnRelay(t *testing.T) {
	h := newTestHub()
	item, _ := h.Put(textInput("durable current", "node1"))
	if !item.ExpiresAt.Equal(item.CreatedAt.Add(mobileHistoryTTL)) {
		t.Fatalf("expected mobile cache expiry hint, got %s", item.ExpiresAt)
	}
	if h.Get() == nil {
		t.Fatal("relay current value must remain until replaced or cleared")
	}
}

func TestGetEmpty(t *testing.T) {
	h := newTestHub()
	if h.Get() != nil {
		t.Fatal("expected nil on empty hub")
	}
}
