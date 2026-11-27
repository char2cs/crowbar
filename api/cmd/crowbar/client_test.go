package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"
)

func TestPostJSON_DecodesErrorEnvelope(t *testing.T) {
	socket := unixTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope[map[string]string]{Success: false, Error: "nope"})
	})
	if _, err := postJSON[map[string]string](socket, "/x"); err == nil {
		t.Fatal("expected an error for a refused envelope")
	}
}

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
