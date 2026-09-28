package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"
)

func TestGetJSON_DecodesSuccessEnvelope(t *testing.T) {
	socket := unixTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope[map[string]string]{Success: true, Data: map[string]string{"k": "v"}})
	})
	got, err := getJSON[map[string]string](socket, "/x")
	if err != nil {
		t.Fatalf("getJSON: %v", err)
	}
	if got["k"] != "v" {
		t.Fatalf("unexpected data: %+v", got)
	}
}

func TestPostJSON_DecodesErrorEnvelope(t *testing.T) {
	socket := unixTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope[map[string]string]{Success: false, Error: "nope"})
	})
	if _, err := postJSON[map[string]string](socket, "/x"); err == nil {
		t.Fatal("expected an error for a refused envelope")
	}
}

// unixTestServer starts an httptest server over a temp unix socket and
// returns the socket path. The socket is created directly under the OS temp
// dir (not t.TempDir()) — its nested per-test subdirectory overflows macOS's
// 104-byte sun_path limit.
func unixTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	f, err := os.CreateTemp("", "cb-*.sock")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	socket := f.Name()
	_ = f.Close()
	if err := os.Remove(socket); err != nil {
		t.Fatalf("remove placeholder: %v", err)
	}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close(); _ = os.Remove(socket) })
	return socket
}
