package tlsid

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_GeneratesOnFirstCall(t *testing.T) {
	dir := t.TempDir()
	id, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if id.Fingerprint == "" {
		t.Fatal("expected a non-empty fingerprint")
	}
	if _, err := os.Stat(filepath.Join(dir, "cert.pem")); err != nil {
		t.Fatalf("expected cert.pem to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "key.pem")); err != nil {
		t.Fatalf("expected key.pem to exist: %v", err)
	}
}

func TestLoad_StableAcrossReloads(t *testing.T) {
	dir := t.TempDir()
	first, err := Load(dir)
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}
	second, err := Load(dir)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprint changed across reloads: %q vs %q", first.Fingerprint, second.Fingerprint)
	}
}

func TestFingerprintOf_Format(t *testing.T) {
	fp := FingerprintOf([]byte("some bytes"))
	if len(fp) < len("SHA256:") || fp[:7] != "SHA256:" {
		t.Fatalf("expected fingerprint to start with SHA256:, got %q", fp)
	}
}
