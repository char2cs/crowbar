package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// `make dev-api` serves TCP while the desktop sidecar serves a unix socket; the
// seed has to reach both off the same --host flag.
func TestNewTransportPicksTCPForATCPHost(t *testing.T) {
	got, err := newTransport("tcp://127.0.0.1:3737")
	if err != nil {
		t.Fatalf("newTransport: %v", err)
	}
	if _, ok := got.(*tcpTransport); !ok {
		t.Fatalf("got %T, want the TCP transport", got)
	}
}

func TestNewTransportPicksTheUnixSocketByDefault(t *testing.T) {
	got, err := newTransport("unix://" + t.TempDir() + "/crowbar.sock")
	if err != nil {
		t.Fatalf("newTransport: %v", err)
	}
	if _, ok := got.(*tcpTransport); ok {
		t.Fatal("a unix:// host must not be served by the TCP transport")
	}
}

func TestTCPTransportGetReturnsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/projects" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	defer srv.Close()

	status, body, err := transportFor(srv).Get(context.Background(), "/v0/projects")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if status != http.StatusOK || !strings.Contains(string(body), `"success":true`) {
		t.Fatalf("status = %d, body = %q", status, body)
	}
}

func TestTCPTransportPostSendsJSONBody(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &received)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	status, _, err := transportFor(srv).PostJSON(
		context.Background(), "/v0/projects", map[string]any{"name": seedProjectName},
	)
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	if received["name"] != seedProjectName {
		t.Fatalf("daemon received %+v", received)
	}
}

func TestTCPTransportPatchSendsJSONBody(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q, want PATCH", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &received)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"c1"}}`))
	}))
	defer srv.Close()

	status, _, err := transportFor(srv).PatchJSON(
		context.Background(), "/v0/chats/c1/branch", map[string]any{"branch": seedFeatureBranch},
	)
	if err != nil {
		t.Fatalf("PatchJSON: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if received["branch"] != seedFeatureBranch {
		t.Fatalf("daemon received %+v", received)
	}
}

// ipc.Client reports a non-2xx as a status with err == nil; the TCP transport
// has to behave identically or the two wires diverge at the call site.
func TestTCPTransportReportsNon2xxAsAStatusNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"success":false,"error":"a workspace already exists for this branch"}`))
	}))
	defer srv.Close()

	status, body, err := transportFor(srv).Get(context.Background(), "/v0/projects")
	if err != nil {
		t.Fatalf("a non-2xx must not surface as a transport error, got %v", err)
	}
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if !strings.Contains(string(body), "already exists") {
		t.Fatalf("body = %q", body)
	}
}

func TestTCPTransportSurfacesADeadDaemon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	wire := transportFor(srv)
	srv.Close()

	if _, _, err := wire.Get(context.Background(), "/v0/projects"); err == nil {
		t.Fatal("expected an unreachable daemon to be a transport error")
	}
}

func transportFor(
	srv *httptest.Server,
) *tcpTransport {
	return newTCPTransport(strings.Replace(srv.URL, "http://", tcpScheme, 1))
}
