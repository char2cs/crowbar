package main

import (
	"strings"
	"testing"
)

func TestDecodeEnvelopeUnwrapsData(t *testing.T) {
	body := []byte(`{"success":true,"data":[{"id":"p1","name":"Crowbar Seed","path":"/tmp/seed"}]}`)

	got, err := decodeEnvelope[[]projectDTO]("list projects", 200, body)
	if err != nil {
		t.Fatalf("decodeEnvelope: %v", err)
	}
	if len(got) != 1 || got[0].ID != "p1" || got[0].Name != seedProjectName {
		t.Fatalf("decoded %+v", got)
	}
}

// PostJSON returns err == nil for a non-2xx, so the status check has to live
// here — otherwise a 404 decodes to a zero value and reads as success.
func TestDecodeEnvelopeRejectsNon2xx(t *testing.T) {
	err := errorFrom[[]repoDTO](t, 404, []byte(`{"success":false,"error":"not found"}`))

	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should name the status and the daemon's message, got %v", err)
	}
}

func TestDecodeEnvelopeRejectsAFailedEnvelopeOn200(t *testing.T) {
	err := errorFrom[workspaceDTO](t, 200, []byte(`{"success":false,"error":"branch is required"}`))

	if !strings.Contains(err.Error(), "branch is required") {
		t.Fatalf("error should carry the daemon's message, got %v", err)
	}
}

func TestDecodeEnvelopeRejectsMalformedJSON(t *testing.T) {
	err := errorFrom[projectDTO](t, 200, []byte(`<!doctype html><html>not the API</html>`))

	if !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("error should name the decode failure, got %v", err)
	}
}

// A 202 answers with an empty body, so decoding one is always a mistake; the
// error has to say the body was empty rather than blame the JSON.
func TestDecodeEnvelopeReportsAnEmptyBody(t *testing.T) {
	err := errorFrom[projectDTO](t, 202, nil)

	if !strings.Contains(err.Error(), "empty body") {
		t.Fatalf("error should name the empty body, got %v", err)
	}
}

func TestSnippetTruncatesALongBody(t *testing.T) {
	got := snippet([]byte(strings.Repeat("x", snippetLimit*2)))

	if len(got) > snippetLimit+len("…") {
		t.Fatalf("snippet is %d bytes, want at most %d", len(got), snippetLimit+len("…"))
	}
}

func errorFrom[T any](
	t *testing.T,
	status int,
	body []byte,
) error {
	t.Helper()
	_, err := decodeEnvelope[T]("seed step", status, body)
	if err == nil {
		t.Fatal("expected an error")
	}
	return err
}
