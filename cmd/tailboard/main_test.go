package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClipboardRequestsDoNotFollowRedirects(t *testing.T) {
	var leaked atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sink.Close()
	for _, code := range []int{301, 302, 303, 307, 308} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", sink.URL+"/api/clip")
				w.WriteHeader(code)
			}))
			defer redirect.Close()
			if err := put(redirect.URL, strings.NewReader("synthetic clipboard")); err == nil {
				t.Fatal("redirect reported success")
			}
			if leaked.Load() != 0 {
				t.Fatal("request escaped the configured destination")
			}
		})
	}
}

func TestClipboardRequestBypassesConfiguredProxy(t *testing.T) {
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.Proxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original; transport.CloseIdleConnections() })
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	if err := request(http.MethodPost, target.URL, bytes.NewBufferString(`{"content":"synthetic"}`), nil); err != nil {
		t.Fatal(err)
	}
	if proxyCalls.Load() != 0 {
		t.Fatal("clipboard request used a system proxy")
	}
}
