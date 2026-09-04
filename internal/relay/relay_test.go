package relay

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostGetAndClear(t *testing.T) {
	var applied string
	cleared := false
	relay := New(func(text string) error { applied = text; return nil }, func() error {
		cleared = true
		return nil
	})
	server := httptest.NewServer(relay.Handler())
	defer server.Close()

	body := bytes.NewBufferString(`{"content":"hello","device_id":"phone"}`)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/clip", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Clip-Source", "Pixel")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated || applied != "hello" {
		t.Fatalf("post status=%d applied=%q", response.StatusCode, applied)
	}

	response, err = http.Get(server.URL + "/api/clip")
	if err != nil {
		t.Fatal(err)
	}
	var item Clip
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if item.Content != "hello" || item.Source != "Pixel" || item.DeviceID != "phone" {
		t.Fatalf("clip = %#v", item)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/clip", nil)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent || !cleared || relay.Current() != nil {
		t.Fatalf("clear status=%d cleared=%t current=%#v", response.StatusCode, cleared, relay.Current())
	}
}

func TestRejectsEmptyAndOversizedText(t *testing.T) {
	relay := New(nil, nil)
	for name, body := range map[string][]byte{
		"empty":     []byte(`{"content":""}`),
		"oversized": append([]byte(`{"content":"`), append(bytes.Repeat([]byte("x"), MaxTextBytes+1), []byte(`"}`)...)...),
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/clip", bytes.NewReader(body))
			response := httptest.NewRecorder()
			relay.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}
}

func TestDuplicateTextDoesNotAdvanceSequence(t *testing.T) {
	relay := New(nil, nil)
	first := relay.PutLocal("same", "Mac")
	second := relay.PutLocal("same", "Mac")
	if first.Seq != 1 || second.Seq != 1 {
		t.Fatalf("sequences = %d, %d", first.Seq, second.Seq)
	}
}
