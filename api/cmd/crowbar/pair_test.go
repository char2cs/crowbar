package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestRunPair_PrintsCodeAndFingerprint(t *testing.T) {
	const fingerprint = "SHA256:aaaa bbbb cccc dddd eeee ffff"
	socket := unixTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope[mintCodeResponse]{
			Success: true,
			Data:    mintCodeResponse{Code: "123456", ServerFingerprint: fingerprint},
		})
	})
	var out bytes.Buffer
	if err := runPair(socket, &out); err != nil {
		t.Fatalf("runPair: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("123456")) {
		t.Fatalf("expected the code in output, got: %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(fingerprint)) {
		t.Fatalf("expected the daemon's fingerprint in output, got: %s", out.String())
	}
}
