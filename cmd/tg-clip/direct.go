package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
)

type directTransferServer struct {
	URL    string
	Token  string
	server *http.Server
}

func startDirectTransferServer(ctx context.Context, address string, paths []string) (*directTransferServer, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		listener.Close()
		return nil, err
	}
	result := &directTransferServer{
		URL:   "http://" + net.JoinHostPort(host, port),
		Token: token,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /files/{index}", func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || index < 0 || index >= len(paths) {
			http.NotFound(w, r)
			return
		}
		file, err := os.Open(paths[index])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(paths[index])))
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	})
	result.server = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = result.server.Serve(listener) }()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = result.server.Shutdown(shutdownCtx)
	}()
	return result, nil
}

func (s *directTransferServer) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func deviceSupportsDirect(device protocol.Device) bool {
	if !device.Online {
		return false
	}
	for _, capability := range device.Capabilities {
		if capability == "direct-fetch" {
			return true
		}
	}
	return false
}
