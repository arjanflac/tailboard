package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/arjanflac/tailboard/internal/tailnet"
)

var version = "dev"

func main() {
	server := flag.String("server", "", "Tailboard URL")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *server == "" {
		if value := os.Getenv("TAILBOARD_SERVER"); value != "" {
			*server = value
		} else if ip, err := tailnet.IPv4(); err == nil {
			*server = "http://" + ip + ":9437"
		}
	}
	if *server == "" || flag.NArg() != 1 {
		usage()
	}

	var err error
	switch flag.Arg(0) {
	case "status":
		err = status(*server)
	case "get":
		err = get(*server)
	case "put":
		err = put(*server, os.Stdin)
	case "clear":
		err = request(http.MethodDelete, *server+"/api/clip", nil, nil)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: tailboard [--server URL] {status|get|put|clear}")
	os.Exit(2)
}

func status(server string) error {
	var values map[string]any
	if err := request(http.MethodGet, server+"/api/status", nil, &values); err != nil {
		return err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%-20s %v\n", key+":", values[key])
	}
	return nil
}

func get(server string) error {
	var clip struct {
		Content string `json:"content"`
	}
	if err := request(http.MethodGet, server+"/api/clip", nil, &clip); err != nil {
		if errors.Is(err, errEmpty) {
			return nil
		}
		return err
	}
	fmt.Print(clip.Content)
	return nil
}

func put(server string, input io.Reader) error {
	text, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(text) == 0 || len(text) > 1<<20 {
		return fmt.Errorf("stdin must contain 1 to 1048576 bytes")
	}
	body, _ := json.Marshal(map[string]string{"content": string(text)})
	return request(http.MethodPost, server+"/api/clip", bytes.NewReader(body), nil)
}

var errEmpty = errors.New("clipboard is empty")

func request(method, url string, body io.Reader, result any) error {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Clip-Source", "CLI")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{
		Timeout:       5 * time.Second,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer transport.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		if result != nil {
			return errEmpty
		}
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("tailboard returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	if result != nil {
		return json.NewDecoder(response.Body).Decode(result)
	}
	return nil
}
