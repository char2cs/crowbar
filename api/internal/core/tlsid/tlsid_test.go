package tlsid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/testutil"
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
	// Verify the exact format: 6 groups of 4 lowercase hex chars separated by spaces.
	parts := strings.Split(fp[7:], " ")
	if len(parts) != 6 {
		t.Fatalf("expected 6 groups separated by spaces, got %d: %q", len(parts), fp)
	}
	for i, part := range parts {
		if len(part) != 4 {
			t.Fatalf("group %d: expected 4 chars, got %d: %q in %q", i, len(part), part, fp)
		}
		for j, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Fatalf("group %d char %d: expected lowercase hex, got %q in %q", i, j, c, fp)
			}
		}
	}
}

func TestLoad_FilePermissions(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	dir := t.TempDir()
	_, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Verify directory permissions are 0o700 (owner-only access).
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if dirPerm := dirInfo.Mode().Perm(); dirPerm != 0o700 {
		t.Fatalf("expected dir perm 0o700, got %#o", dirPerm)
	}
	// Verify key.pem permissions are 0o600 (owner read-write only).
	keyPath := filepath.Join(dir, "key.pem")
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key.pem: %v", err)
	}
	if keyPerm := keyInfo.Mode().Perm(); keyPerm != 0o600 {
		t.Fatalf("expected key.pem perm 0o600, got %#o", keyPerm)
	}
}
