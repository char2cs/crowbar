# Remote Instances: Daemon Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the Go daemon (`api/`) everything it needs to authenticate a client over the network — TLS termination, a CLI-only pairing-code flow, a permanent-revocation client registry — with zero knowledge of any *other* daemon.

**Architecture:** The daemon gains a stable self-signed TLS identity (`tlsid`), a loopback-only pairing-code mint + network-reachable rate-limited redeem endpoint (`pairing` usecase), a GORM-backed client registry keyed by a hashed bearer token (`domain.Client` + `GORMStores.Clients`), and a `requirePairedClient` middleware that gates every route except the pairing routes themselves and anything reached over a non-TLS connection (unix socket / loopback dev TCP). A new `GET /v0/system/browse` endpoint lists an arbitrary directory so a remote client can pick a project root.

**Tech Stack:** Go, Gin, GORM, `github.com/google/uuid`, `github.com/spf13/cobra`.

**Spec:** `docs/superpowers/specs/2026-09-27-remote-instances-design.md` (§3 Auth & transport, §5.3 browse endpoint's client, §7 testing).

## Global Constraints

- Build and test with `-tags noEmbed`; lint with `make lint` (new code) and `make lint-backlog`; `make deadcode` for the dead-code gate.
- One test file per source file (`foo.go` → `foo_test.go`), struct-only files excepted.
- Source files under 500 lines.
- Early returns over `else`; at most three indentation levels per function.
- Wrap errors with context: `fmt.Errorf("op: ctx: %w", err)`; check every error.
- Tests are deterministic: no `time.Sleep` — wait on a channel, a WaitGroup, or a condition with a deadline.
- No `...ForTest` methods or test-only package vars in non-test files; use `export_test.go` or an injected config/clock.
- golangci-lint: funlen 100 lines/50 statements, gocyclo 15, nestif 2, revive early-return, gofumpt.
- This is **server-TLS-only, bearer-token auth** — NOT mTLS. No client certificate is ever requested or required. (A prior, discarded design on this branch used mTLS; do not resurrect client-cert code from git history without re-reading the current spec.)

## Review Focus

- Two redeem requests racing the same pairing code must not both succeed, leak the code as still-outstanding, or double-register a client.
- A revoked client's very next request (same process, no restart) must be denied — the auth check must re-read the registry per request, never cache a decision.
- An empty, garbage, or well-formed-but-unknown bearer token must produce a clean 401, never a panic or 500.
- `GET /v0/system/browse` given a nonexistent path, an unreadable path, or a path that names a file (not a directory) must produce a clean 4xx, never a 500.
- The pairing-token mint endpoint must refuse any request arriving over TLS (`c.Request.TLS != nil`) — this is the boundary that stops a network caller from minting its own credential — and must succeed for a plaintext (unix socket or loopback dev TCP) request.

---

### Task 1: `tlsid` — daemon TLS identity

**Files:**
- Create: `api/internal/core/tlsid/tlsid.go`
- Test: `api/internal/core/tlsid/tlsid_test.go`

**Interfaces:**
- Produces: `tlsid.Identity{Certificate tls.Certificate, Fingerprint string}`, `tlsid.Load(dir string) (*Identity, error)`, `tlsid.FingerprintOf(der []byte) string`.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd api && go test -tags noEmbed ./internal/core/tlsid/... -v`
Expected: FAIL — package `tlsid` does not exist yet.

- [ ] **Step 3: Implement `tlsid`**

```go
// Package tlsid owns the daemon's long-lived TLS identity: a self-signed
// certificate generated once and reused for the life of the installation.
//
// Stability is the whole point. A client pairs by confirming this identity's
// fingerprint, so regenerating it silently breaks every paired client — the
// certificate is created only when absent and never rotated automatically.
package tlsid

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Identity is the daemon's TLS keypair plus the fingerprint a human confirms
// during pairing.
type Identity struct {
	Certificate tls.Certificate
	Fingerprint string
}

const (
	certName = "cert.pem"
	keyName  = "key.pem"
	validFor = 10 * 365 * 24 * time.Hour
)

// Load reads the identity in dir, generating it if absent.
func Load(dir string) (*Identity, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("tlsid: create dir: %w", err)
	}
	//nolint:gosec // the TLS identity directory must not be group/world readable
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("tlsid: restrict dir: %w", err)
	}
	certPath := filepath.Join(dir, certName)
	keyPath := filepath.Join(dir, keyName)

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		if err := generate(certPath, keyPath); err != nil {
			return nil, err
		}
	}

	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("tlsid: load keypair: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return nil, fmt.Errorf("tlsid: keypair carries no certificate")
	}
	return &Identity{Certificate: pair, Fingerprint: FingerprintOf(pair.Certificate[0])}, nil
}

// FingerprintOf renders a certificate's SHA-256 as grouped lowercase hex.
func FingerprintOf(der []byte) string {
	sum := sha256.Sum256(der)
	full := hex.EncodeToString(sum[:])
	groups := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		groups = append(groups, full[i*4:(i+1)*4])
	}
	return "SHA256:" + strings.Join(groups, " ")
}

func generate(certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("tlsid: generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("tlsid: serial: %w", err)
	}

	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "crowbar"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              []string{"crowbar", "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("tlsid: create certificate: %w", err)
	}

	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("tlsid: marshal key: %w", err)
	}
	return writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600)
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	buf := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if err := os.WriteFile(path, buf, mode); err != nil {
		return fmt.Errorf("tlsid: write %s: %w", filepath.Base(path), err)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/core/tlsid/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/core/tlsid/
git commit -m "feat(api): daemon TLS identity (tlsid)"
```

---

### Task 2: `domain.Client` + GORM store

**Files:**
- Create: `api/internal/domain/client.go`
- Modify: `api/internal/app/gorm.go` (add `Clients` field to `GORMStores` and its construction in `newGORMStores`)
- Test: `api/internal/app/gorm_test.go` (extend the existing file with a `Clients` case)

**Interfaces:**
- Produces: `domain.Client{ID, TokenHash, Name, PairedAt, LastSeen, RevokedAt}`; `GORMStores.Clients store.ScopedStore[domain.Client, string]`.
- Consumes: `store.ScopedStore[T, K]` (`api/internal/adapter/store/store.go`) — `Save`, `Delete`, `FindByKey`, `FindAll`, `FindWhere(ctx, match T) ([]T, error)` (matches non-zero fields of `match`, conjunctively).

- [ ] **Step 1: Write `domain.Client`**

```go
package domain

import "time"

// Client is a device paired to talk to this daemon over the network. The
// credential is a bearer token; only its SHA-256 hash is ever persisted —
// the raw token is shown to the pairing client exactly once, at redeem time,
// and never stored anywhere.
type Client struct {
	ID string `gorm:"primaryKey" json:"id"`
	// TokenHash is the SHA-256 hex digest of the bearer token this client
	// presents on every request. Unique: two clients never share a token.
	TokenHash string     `gorm:"uniqueIndex" json:"-"`
	Name      string     `json:"name"`
	PairedAt  time.Time  `json:"pairedAt"`
	LastSeen  time.Time  `json:"lastSeen"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

func (Client) TableName() string {
	return "clients"
}
```

- [ ] **Step 2: Read `api/internal/app/gorm.go` in full**

Read the file before editing. Confirm the exact shape of one existing field (e.g. `Folders store.ScopedStore[domain.Folder, string]`) and its construction line in `newGORMStores` (`storesqlite.NewFromDB[domain.Folder, string](db)` or equivalent) — mirror that exact pattern for `Clients`, since this plan was written against a snapshot of the file and the surrounding lines are the source of truth.

- [ ] **Step 3: Add `Clients` to `GORMStores`**

Add a field to the `GORMStores` struct:

```go
	Clients                  store.ScopedStore[domain.Client, string]
```

And in `newGORMStores`, following the exact pattern the file already uses for `Folders`/`Repositories` (construct-and-check-error, then reference in the returned struct literal):

```go
	clients, err := storesqlite.NewFromDB[domain.Client, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: client store: %w", err)
	}
```

...and add `Clients: clients,` to the struct literal `newGORMStores` returns.

- [ ] **Step 4: Write the failing test**

Add to `api/internal/app/gorm_test.go` (following the file's existing test style for another store — read one existing test in that file first and mirror its setup/teardown):

```go
func TestGORMStores_Clients_SaveFindRevoke(t *testing.T) {
	stores := newTestGORMStores(t) // use whatever helper the existing tests in this file already use to build a *GORMStores over a temp DB
	ctx := context.Background()

	c := domain.Client{
		ID:        "client-1",
		TokenHash: "deadbeef",
		Name:      "test-device",
		PairedAt:  time.Now(),
		LastSeen:  time.Now(),
	}
	if err := stores.Clients.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := stores.Clients.FindWhere(ctx, domain.Client{TokenHash: "deadbeef"})
	if err != nil {
		t.Fatalf("FindWhere: %v", err)
	}
	if len(found) != 1 || found[0].ID != "client-1" {
		t.Fatalf("expected to find client-1 by token hash, got %+v", found)
	}

	now := time.Now()
	found[0].RevokedAt = &now
	if err := stores.Clients.Save(ctx, found[0]); err != nil {
		t.Fatalf("Save (revoke): %v", err)
	}

	revoked, err := stores.Clients.FindByKey(ctx, "client-1")
	if err != nil {
		t.Fatalf("FindByKey: %v", err)
	}
	if revoked == nil || revoked.RevokedAt == nil {
		t.Fatal("expected RevokedAt to be set")
	}
}
```

Adjust `newTestGORMStores(t)` to whatever helper name the existing file actually uses.

- [ ] **Step 5: Run to verify it fails, then implement, then verify it passes**

Run: `cd api && go test -tags noEmbed ./internal/app/... -run TestGORMStores_Clients -v`
Expected first: FAIL (`Clients` field does not exist). After Step 3's edit: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/domain/client.go api/internal/app/gorm.go api/internal/app/gorm_test.go
git commit -m "feat(api): domain.Client and a GORM-backed client registry"
```

---

### Task 3: `pairing` usecase — mint & redeem

**Files:**
- Create: `api/internal/app/usecases/pairing/pairing.go`
- Test: `api/internal/app/usecases/pairing/pairing_test.go`

**Interfaces:**
- Consumes: `store.ScopedStore[domain.Client, string]` (Task 2) as the client-persistence seam (narrowed to a local interface, see below).
- Produces: `pairing.Usecase{Mint(now time.Time) (code string, err error), Redeem(ctx, code, name string, now time.Time) (token string, client domain.Client, err error)}`, `pairing.New(clients ClientStore) Usecase`, `pairing.HashToken(token string) string` (exported so Task 4's middleware can hash an incoming bearer token the same way without depending on the rest of this package), `pairing.CodeTTL` (5 * time.Minute), errors `ErrNoCode`, `ErrBadCode`, `ErrCodeExpired`.

- [ ] **Step 1: Write the failing tests**

```go
package pairing

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeClientStore struct {
	saved []domain.Client
}

func (f *fakeClientStore) Save(_ context.Context, c domain.Client) error {
	f.saved = append(f.saved, c)
	return nil
}

func TestMint_ReturnsSixDigitCode(t *testing.T) {
	u := New(&fakeClientStore{})
	code, err := u.Mint(time.Now())
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("expected a 6-digit code, got %q", code)
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			t.Fatalf("expected all digits, got %q", code)
		}
	}
}

func TestRedeem_ValidCode_CreatesClientAndReturnsToken(t *testing.T) {
	store := &fakeClientStore{}
	u := New(store)
	now := time.Now()
	code, err := u.Mint(now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	token, client, err := u.Redeem(context.Background(), code, "my-laptop", now)
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty bearer token")
	}
	if client.Name != "my-laptop" {
		t.Fatalf("expected client name my-laptop, got %q", client.Name)
	}
	if client.TokenHash != HashToken(token) {
		t.Fatal("persisted TokenHash does not match the returned token's hash")
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected exactly one client persisted, got %d", len(store.saved))
	}
}

func TestRedeem_WrongCode_Fails(t *testing.T) {
	u := New(&fakeClientStore{})
	now := time.Now()
	if _, err := u.Mint(now); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if _, _, err := u.Redeem(context.Background(), "000000", "x", now); err == nil {
		t.Fatal("expected an error for a wrong code")
	}
}

func TestRedeem_NoCodeOutstanding_Fails(t *testing.T) {
	u := New(&fakeClientStore{})
	if _, _, err := u.Redeem(context.Background(), "123456", "x", time.Now()); err == nil {
		t.Fatal("expected an error when no code was minted")
	}
}

func TestRedeem_ExpiredCode_Fails(t *testing.T) {
	u := New(&fakeClientStore{})
	now := time.Now()
	code, err := u.Mint(now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if _, _, err := u.Redeem(context.Background(), code, "x", now.Add(CodeTTL+time.Second)); err == nil {
		t.Fatal("expected an error for an expired code")
	}
}

func TestRedeem_IsSingleUse(t *testing.T) {
	store := &fakeClientStore{}
	u := New(store)
	now := time.Now()
	code, err := u.Mint(now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if _, _, err := u.Redeem(context.Background(), code, "first", now); err != nil {
		t.Fatalf("first Redeem: %v", err)
	}
	if _, _, err := u.Redeem(context.Background(), code, "second", now); err == nil {
		t.Fatal("expected the second redeem of the same code to fail")
	}
}

func TestHashToken_IsStableAndDistinct(t *testing.T) {
	if HashToken("a") != HashToken("a") {
		t.Fatal("HashToken must be deterministic")
	}
	if HashToken("a") == HashToken("b") {
		t.Fatal("HashToken must distinguish different inputs")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/pairing/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement**

```go
// Package pairing mints single-use, human-typeable pairing codes and redeems
// them for a bearer token a client presents on every later request.
package pairing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
)

var (
	// ErrNoCode signals no code is outstanding.
	ErrNoCode = errors.New("pairing: no code outstanding")
	// ErrBadCode signals the provided code does not match.
	ErrBadCode = errors.New("pairing: code does not match")
	// ErrCodeExpired signals the code has expired.
	ErrCodeExpired = errors.New("pairing: code expired")
)

// CodeTTL is the time-to-live for a minted pairing code.
const CodeTTL = 5 * time.Minute

// ClientStore is the narrow persistence seam this package needs — satisfied
// by store.ScopedStore[domain.Client, string] (api/internal/app/gorm.go).
type ClientStore interface {
	Save(ctx context.Context, c domain.Client) error
}

// Usecase mints and redeems pairing codes.
type Usecase interface {
	Mint(now time.Time) (code string, err error)
	Redeem(ctx context.Context, code, name string, now time.Time) (token string, client domain.Client, err error)
}

type usecase struct {
	clients ClientStore

	mu      sync.Mutex
	code    string
	expires time.Time
}

// New builds the pairing usecase.
func New(clients ClientStore) Usecase {
	return &usecase{clients: clients}
}

// Mint generates a new pairing code and invalidates any previous one.
func (u *usecase) Mint(now time.Time) (string, error) {
	code, err := newCode()
	if err != nil {
		return "", err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.code = code
	u.expires = now.Add(CodeTTL)
	return code, nil
}

// Redeem consumes the code, mints a bearer token, and persists the client
// keyed by the token's hash. The raw token is returned exactly once — it is
// never itself persisted.
func (u *usecase) Redeem(
	ctx context.Context,
	code, name string,
	now time.Time,
) (string, domain.Client, error) {
	claimed, claimedExp, err := u.claim(code, now)
	if err != nil {
		return "", domain.Client{}, err
	}

	token, err := newToken()
	if err != nil {
		u.restore(claimed, claimedExp)
		return "", domain.Client{}, err
	}

	c := domain.Client{
		ID:        uuid.NewString(),
		TokenHash: HashToken(token),
		Name:      name,
		PairedAt:  now,
		LastSeen:  now,
	}
	if err := u.clients.Save(ctx, c); err != nil {
		u.restore(claimed, claimedExp)
		return "", domain.Client{}, fmt.Errorf("pairing: persist client: %w", err)
	}
	return token, c, nil
}

// claim validates the code and takes it in one critical section, so a second
// caller cannot observe it as still outstanding.
func (u *usecase) claim(code string, now time.Time) (string, time.Time, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.code == "" {
		return "", time.Time{}, ErrNoCode
	}
	// The code is a bearer secret; constant-time compare avoids a timing oracle.
	if subtle.ConstantTimeCompare([]byte(u.code), []byte(code)) != 1 {
		// Deliberately does not burn: a wrong guess must not lock out the operator.
		return "", time.Time{}, ErrBadCode
	}
	if now.After(u.expires) {
		return "", time.Time{}, ErrCodeExpired
	}

	claimed, claimedExp := u.code, u.expires
	u.code, u.expires = "", time.Time{}
	return claimed, claimedExp, nil
}

// restore puts a claimed code back after a failed write, unless a newer code
// has been minted in the meantime.
func (u *usecase) restore(code string, expires time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.code == "" {
		u.code, u.expires = code, expires
	}
}

// newCode returns a 6-digit numeric pairing code.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("pairing: entropy: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// newToken returns a 32-byte bearer token, hex-encoded.
func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("pairing: token entropy: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// HashToken is the one place a bearer token's persisted form is computed, so
// the redeem path and the auth middleware never drift.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run to verify the tests pass**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/pairing/... -v`
Expected: PASS, including `TestRedeem_IsSingleUse` — this is the Review Focus item about a racing/replayed redeem.

- [ ] **Step 5: Add a concurrency test for the Review Focus item on racing redeems**

```go
func TestRedeem_ConcurrentRedeemsOnlyOneSucceeds(t *testing.T) {
	store := &fakeClientStore{}
	u := New(store)
	now := time.Now()
	code, err := u.Mint(now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	const attempts = 20
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := u.Redeem(context.Background(), code, "racer", now)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 successful redeem of a racing code, got %d", successes)
	}
}
```

Add `"sync"` to the test file's imports. Run with the race detector: `cd api && go test -tags noEmbed ./internal/app/usecases/pairing/... -race -v`. Expected: PASS, no data race reported.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/usecases/pairing/
git commit -m "feat(api): pairing usecase — mint/redeem a 6-digit code for a bearer token"
```

---

### Task 4: bearer-auth middleware + redeem rate limiter

**Files:**
- Modify: `api/internal/api/v0/middleware.go` (read the file in full first — it already has `scopeRepoToPath`/`scopeWorkspaceToPath`/`rejectEmptyPathParams`; add alongside them)
- Create: `api/internal/api/v0/ratelimit.go`
- Test: `api/internal/api/v0/middleware_test.go` (extend the existing file), `api/internal/api/v0/ratelimit_test.go`

**Interfaces:**
- Consumes: a narrow client-lookup seam `clientAuthStore interface { FindWhere(ctx, domain.Client) ([]domain.Client, error) }` (satisfied by `store.ScopedStore[domain.Client, string]`), `pairing.HashToken` (Task 3).
- Produces: `requireBearerAuth(clients clientAuthStore) gin.HandlerFunc`, `ClientIDFrom(c *gin.Context) string`, `newFixedWindowLimiter(max int, window time.Duration) gin.HandlerFunc`.

- [ ] **Step 1: Write the failing middleware test**

Append to `middleware_test.go` (mirror the file's existing `httptest`/gin test-harness style — read one existing test in the file first):

```go
type stubClientStore struct {
	clients []domain.Client
}

func (s *stubClientStore) FindWhere(_ context.Context, match domain.Client) ([]domain.Client, error) {
	var out []domain.Client
	for _, c := range s.clients {
		if match.TokenHash != "" && c.TokenHash == match.TokenHash {
			out = append(out, c)
		}
	}
	return out, nil
}

func TestRequireBearerAuth_NoTLS_PassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil) // no TLS
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a non-TLS request, got %d", rec.Code)
	}
}

func TestRequireBearerAuth_TLS_NoHeader_Rejects(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no Authorization header, got %d", rec.Code)
	}
}

func TestRequireBearerAuth_TLS_UnknownToken_Rejects(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an unknown token, got %d", rec.Code)
	}
}

func TestRequireBearerAuth_TLS_ValidToken_Passes(t *testing.T) {
	token := "abc123"
	store := &stubClientStore{clients: []domain.Client{
		{ID: "c1", TokenHash: pairing.HashToken(token)},
	}}
	r := gin.New()
	r.Use(requireBearerAuth(store))
	r.GET("/x", func(c *gin.Context) {
		if ClientIDFrom(c) != "c1" {
			t.Errorf("expected ClientIDFrom to return c1, got %q", ClientIDFrom(c))
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a valid token, got %d", rec.Code)
	}
}

func TestRequireBearerAuth_TLS_RevokedToken_Rejects(t *testing.T) {
	token := "abc123"
	revokedAt := time.Now()
	store := &stubClientStore{clients: []domain.Client{
		{ID: "c1", TokenHash: pairing.HashToken(token), RevokedAt: &revokedAt},
	}}
	r := gin.New()
	r.Use(requireBearerAuth(store))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a revoked client's token (Review Focus: revoke takes effect immediately), got %d", rec.Code)
	}
}
```

Add `"context"`, `"crypto/tls"`, `"net/http/httptest"`, `"time"`, and the `pairing`/`domain` imports to the test file's import block, matching whatever aliasing the file already uses.

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/... -run TestRequireBearerAuth -v`
Expected: FAIL — `requireBearerAuth`/`ClientIDFrom` undefined.

- [ ] **Step 3: Implement `requireBearerAuth` in `middleware.go`**

Add to the existing file (do not remove `scopeRepoToPath`/`scopeWorkspaceToPath`/`rejectEmptyPathParams` — this is additive):

```go
// ctxClientKey is where requireBearerAuth records the authenticated client's
// id for downstream handlers.
const ctxClientKey = "crowbar.clientID"

// ClientIDFrom returns the authenticated client's id, or "" for a request
// that arrived over a non-TLS connection (the unix socket, or a loopback dev
// TCP bind, which stays plaintext by design).
func ClientIDFrom(c *gin.Context) string {
	v, _ := c.Get(ctxClientKey)
	id, _ := v.(string)
	return id
}

// clientAuthStore is the narrow seam requireBearerAuth needs — satisfied by
// store.ScopedStore[domain.Client, string].
type clientAuthStore interface {
	FindWhere(ctx context.Context, match domain.Client) ([]domain.Client, error)
}

// requireBearerAuth authenticates a request by its Authorization: Bearer
// header. A request with no TLS state (the unix socket, or a loopback dev TCP
// bind) passes through unauthenticated — filesystem permission on the socket
// is that path's trust boundary, not this middleware.
func requireBearerAuth(clients clientAuthStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.TLS == nil {
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
			libs.WriteErr(c, http.StatusUnauthorized, "client is not paired")
			c.Abort()
			return
		}
		token := auth[len(prefix):]
		matches, err := clients.FindWhere(c.Request.Context(), domain.Client{TokenHash: pairing.HashToken(token)})
		if err != nil {
			libs.WriteErr(c, http.StatusInternalServerError, "client lookup failed")
			c.Abort()
			return
		}
		if len(matches) == 0 || matches[0].RevokedAt != nil {
			libs.WriteErr(c, http.StatusUnauthorized, "client is not paired")
			c.Abort()
			return
		}
		c.Set(ctxClientKey, matches[0].ID)
		c.Next()
	}
}
```

Add `"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"` to the file's imports.

- [ ] **Step 4: Run to verify the middleware tests pass**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/... -run TestRequireBearerAuth -v`
Expected: PASS.

- [ ] **Step 5: Write the failing rate-limiter test**

```go
package v0

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestFixedWindowLimiter_AllowsUpToMax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newFixedWindowLimiter(3, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
}

func TestFixedWindowLimiter_RejectsOverMax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newFixedWindowLimiter(3, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on the 4th request within the window, got %d", rec.Code)
	}
}

func TestFixedWindowLimiter_ResetsAfterWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newFixedWindowLimiter(1, 10*time.Millisecond))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	rejected := httptest.NewRecorder()
	r.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rejected.Code != http.StatusTooManyRequests {
		t.Fatal("expected the 2nd immediate request to be rate-limited")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code == http.StatusOK {
			return // window reset, as expected
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("limiter never reset within 1s of a 10ms window")
}
```

- [ ] **Step 6: Run to verify failure, then implement `ratelimit.go`**

```go
package v0

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// newFixedWindowLimiter rejects requests past max within a single rolling
// window, reset (not decayed) once the window elapses. It is global, not
// per-caller: the pairing-redeem endpoint has no identity to key on before a
// caller is paired, and a global cap on wrong guesses is the point.
func newFixedWindowLimiter(max int, window time.Duration) gin.HandlerFunc {
	var (
		mu         sync.Mutex
		count      int
		windowEnds time.Time
	)
	return func(c *gin.Context) {
		mu.Lock()
		now := time.Now()
		if now.After(windowEnds) {
			count = 0
			windowEnds = now.Add(window)
		}
		count++
		over := count > max
		mu.Unlock()

		if over {
			libs.WriteErr(c, http.StatusTooManyRequests, "too many requests, try again shortly")
			c.Abort()
			return
		}
		c.Next()
	}
}
```

Run: `cd api && go test -tags noEmbed ./internal/api/v0/... -run TestFixedWindowLimiter -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/api/v0/middleware.go api/internal/api/v0/middleware_test.go api/internal/api/v0/ratelimit.go api/internal/api/v0/ratelimit_test.go
git commit -m "feat(api): bearer-token auth middleware and a fixed-window rate limiter"
```

---

### Task 5: pairing HTTP handlers, routes, and router wiring

**Files:**
- Create: `api/internal/api/v0/endpoints/pairing/handlers/handlers.go`
- Create: `api/internal/api/v0/endpoints/pairing/handlers/handlers_test.go`
- Create: `api/internal/api/v0/endpoints/pairing/routes.go`
- Modify: `api/internal/api/v0/router.go` (read in full first; mount the pairing routes)
- Modify: `api/internal/api/v0/container.go` (read in full first; construct the `pairing.Usecase` and the handlers)

**Interfaces:**
- Consumes: `pairing.Usecase` (Task 3), `store.ScopedStore[domain.Client, string]` (Task 2, for list/revoke), `requireBearerAuth` (Task 4), `newFixedWindowLimiter` (Task 4).
- Produces: `POST /v0/pair/code` (mint, loopback/non-TLS only), `POST /v0/pair` (redeem, rate-limited), `GET /v0/clients` (guarded), `DELETE /v0/clients/:id` (guarded, permanent revoke).

- [ ] **Step 1: Write the failing handler tests**

```go
// Package handlers_test exercises the pairing HTTP surface end to end
// against gin's test harness, with a fake pairing.Usecase and client store —
// no real DB.
package handlers_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakePairing struct {
	mintCode   string
	mintErr    error
	redeemErr  error
	redeemTok  string
	redeemName string
}

func (f *fakePairing) Mint(time.Time) (string, error) { return f.mintCode, f.mintErr }
func (f *fakePairing) Redeem(_ context.Context, code, name string, _ time.Time) (string, domain.Client, error) {
	f.redeemName = name
	if f.redeemErr != nil {
		return "", domain.Client{}, f.redeemErr
	}
	return f.redeemTok, domain.Client{ID: "c1", Name: name}, nil
}

type fakeRegistry struct {
	clients []domain.Client
	revoked string
}

func (f *fakeRegistry) FindAll(context.Context) ([]domain.Client, error) { return f.clients, nil }
func (f *fakeRegistry) Save(_ context.Context, c domain.Client) error {
	f.revoked = c.ID
	return nil
}
func (f *fakeRegistry) FindByKey(_ context.Context, id string) (*domain.Client, error) {
	for _, c := range f.clients {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, nil
}

func newTestRouter(pairingUC *fakePairing, registry *fakeRegistry) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handlers.New(pairingUC, registry)
	open := r.Group("")
	guarded := r.Group("")
	handlers.Register(open, guarded, h) // signature under test — see routes.go below
	return r
}

func TestMintCode_ReturnsCode(t *testing.T) {
	r := newTestRouter(&fakePairing{mintCode: "123456"}, &fakeRegistry{})
	req := httptest.NewRequest(http.MethodPost, "/pair/code", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Code != "123456" {
		t.Fatalf("expected code 123456, got %q", body.Data.Code)
	}
}

func TestMintCode_OverTLS_Returns403(t *testing.T) {
	// Review Focus: minting must be refused for any request that arrived over
	// TLS — only a plaintext (unix socket / loopback dev TCP) caller may mint.
	r := newTestRouter(&fakePairing{mintCode: "123456"}, &fakeRegistry{})
	req := httptest.NewRequest(http.MethodPost, "/pair/code", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a mint request over TLS, got %d", rec.Code)
	}
}

func TestPair_ValidCode_ReturnsToken(t *testing.T) {
	r := newTestRouter(&fakePairing{redeemTok: "sometoken"}, &fakeRegistry{})
	reqBody, _ := json.Marshal(map[string]string{"code": "123456", "name": "my-device"})
	req := httptest.NewRequest(http.MethodPost, "/pair", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Token    string `json:"token"`
			ClientID string `json:"clientId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Token != "sometoken" || body.Data.ClientID != "c1" {
		t.Fatalf("unexpected body: %+v", body.Data)
	}
}

func TestPair_BadCode_Returns401(t *testing.T) {
	r := newTestRouter(&fakePairing{redeemErr: errBadCodeStub}, &fakeRegistry{})
	reqBody, _ := json.Marshal(map[string]string{"code": "000000", "name": "x"})
	req := httptest.NewRequest(http.MethodPost, "/pair", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestListClients_ReturnsAll(t *testing.T) {
	r := newTestRouter(&fakePairing{}, &fakeRegistry{clients: []domain.Client{{ID: "c1"}, {ID: "c2"}}})
	req := httptest.NewRequest(http.MethodGet, "/clients", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRevokeClient_MarksRevoked(t *testing.T) {
	registry := &fakeRegistry{clients: []domain.Client{{ID: "c1"}}}
	r := newTestRouter(&fakePairing{}, registry)
	req := httptest.NewRequest(http.MethodDelete, "/clients/c1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if registry.revoked != "c1" {
		t.Fatal("expected client c1 to be revoked")
	}
}

var errBadCodeStub = &stubErr{"bad code"}

type stubErr struct{ msg string }

func (e *stubErr) Error() string { return e.msg }
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/endpoints/pairing/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `handlers.go`**

```go
// Package handlers serves the pairing and client-registry HTTP routes.
package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Pairing is the narrow usecase surface these handlers need.
type Pairing interface {
	Mint(now time.Time) (code string, err error)
	Redeem(ctx context.Context, code, name string, now time.Time) (token string, client domain.Client, err error)
}

// Registry is the narrow client-registry surface the list/revoke routes need.
type Registry interface {
	FindAll(ctx context.Context) ([]domain.Client, error)
	FindByKey(ctx context.Context, id string) (*domain.Client, error)
	Save(ctx context.Context, c domain.Client) error
}

// mintCodeResponse is the body of POST /pair/code.
type mintCodeResponse struct {
	Code string `json:"code"`
}

// pairRequest is the body of POST /pair.
type pairRequest struct {
	Code string `json:"code" binding:"required"`
	Name string `json:"name"`
}

// pairResponse is the body of a successful POST /pair.
type pairResponse struct {
	Token    string `json:"token"`
	ClientID string `json:"clientId"`
}

// Handlers serves pairing and client management.
type Handlers struct {
	pairing  Pairing
	registry Registry
}

// New builds the handler set.
func New(p Pairing, registry Registry) *Handlers {
	return &Handlers{pairing: p, registry: registry}
}

// MintCode issues a fresh pairing code. Callable only over a non-TLS
// connection — the unix socket, or a loopback dev TCP bind — which is the
// whole reason a redeem over the network can trust it (Review Focus: this
// endpoint must refuse a TLS caller and succeed for a plaintext one).
func (h *Handlers) MintCode(c *gin.Context) {
	if c.Request.TLS != nil {
		libs.WriteErr(c, http.StatusForbidden, "pairing codes can only be minted over the local socket")
		return
	}
	code, err := h.pairing.Mint(time.Now())
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteQueryOK(c, mintCodeResponse{Code: code})
}

// Pair redeems a pairing code for a bearer token.
func (h *Handlers) Pair(c *gin.Context) {
	var req pairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	token, client, err := h.pairing.Redeem(c.Request.Context(), req.Code, req.Name, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, pairing.ErrNoCode), errors.Is(err, pairing.ErrBadCode), errors.Is(err, pairing.ErrCodeExpired):
			libs.WriteErr(c, http.StatusUnauthorized, "pairing code is not valid")
		default:
			libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	libs.WriteQueryOK(c, pairResponse{Token: token, ClientID: client.ID})
}

// ListClients returns every paired client.
func (h *Handlers) ListClients(c *gin.Context) {
	out, err := h.registry.FindAll(c.Request.Context())
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteQueryOK(c, out)
}

// RevokeClient permanently revokes one client. No later pairing code
// reactivates a revoked id (Review Focus / spec §3): a fresh pairing always
// mints a fresh client.
func (h *Handlers) RevokeClient(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.registry.FindByKey(c.Request.Context(), id)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		libs.WriteErr(c, http.StatusNotFound, "client not found")
		return
	}
	now := time.Now()
	existing.RevokedAt = &now
	if err := h.registry.Save(c.Request.Context(), *existing); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteQueryOK(c, gin.H{"revoked": true})
}
```

Check `libs.WriteQueryOK`'s exact name in `api/internal/api/libs/` before writing this — it is used elsewhere in the codebase as `libs.WriteQueryOK(c, data)`; if the current package spells it differently, use the real name.

- [ ] **Step 4: Implement `routes.go`**

```go
// Package pairing mounts the pairing and client-registry routes.
package pairing

import (
	"github.com/gin-gonic/gin"

	pairinghandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
)

// Register mounts the routes. open carries no auth guard — minting and
// redeeming are how an unpaired client becomes paired, so they cannot
// require the very credential they produce. guarded carries requireBearerAuth
// like every other v0 route. The redeem route additionally needs the
// fixed-window rate limiter applied by the caller (router.go), since it has
// no client identity to key on before pairing succeeds.
func Register(
	open *gin.RouterGroup,
	guarded *gin.RouterGroup,
	h *pairinghandlers.Handlers,
) {
	open.POST("/pair/code", h.MintCode)
	open.POST("/pair", h.Pair)
	guarded.GET("/clients", h.ListClients)
	guarded.DELETE("/clients/:id", h.RevokeClient)
}
```

- [ ] **Step 5: Run to verify the handler+routes tests pass**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/endpoints/pairing/... -v`
Expected: PASS.

- [ ] **Step 6: Wire into `router.go` and `container.go`**

Read both files in full first (their current content is summarized above in this plan's research, but re-read before editing — they are large, hand-tuned files). In `container.go`, construct `pairing.New(appContainer.GORM.Clients)` and `pairinghandlers.New(pairingUC, appContainer.GORM.Clients)`, storing the handlers on the `Container` struct alongside the other endpoint handler instances (e.g. `pairing *pairinghandlers.Handlers`).

In `router.go`'s `Register(rg *gin.RouterGroup)`, mount the pairing routes and the auth guard in this exact order (order is load-bearing — see the comment):

```go
	// Pairing's OPEN routes are registered on rg BEFORE requireBearerAuth is
	// installed below — gin copies a group's middleware into a route at
	// REGISTRATION time, so anything registered before rg.Use(...) never
	// carries it. This is what lets an unpaired client redeem a code at all.
	pairingPkg.Register(rg, rg, c.pairing) // 'open' and 'guarded' both = rg here; the guard is added AFTER
	// Health must stay reachable before pairing too — the pairing dialog's
	// own first stage health-checks a host it hasn't paired with yet.
	health.Register(rg)

	rg.Use(rejectEmptyPathParams())
	// From here down, every route requires a valid bearer token OVER TLS —
	// and is a no-op over the unix socket / loopback dev TCP, per
	// requireBearerAuth's own doc.
	rg.Use(requireBearerAuth(c.app.GORM.Clients))
```

This replaces the plan's earlier two-group idea (`open`/`guarded` as genuinely separate groups) with the simpler registration-order trick, since `Register(rg *gin.RouterGroup)` only receives one group here — but note `pairingPkg.Register`'s own signature still takes two params for symmetry with its own tests (Step 4 above); passing `rg, rg` at this one call site is fine. Apply the fixed-window rate limiter to the redeem route specifically, not globally — after mounting, add:

```go
	// Wrap POST /pair specifically: 5 attempts/minute, global (no identity to
	// key on before pairing succeeds) — a wrong-code guesser is capped without
	// capping the health probe every saved host runs continuously.
```

then adjust `pairing.Register`'s open group registration to apply `newFixedWindowLimiter(5, time.Minute)` to `/pair` only (e.g. `open.POST("/pair", newFixedWindowLimiter(5, time.Minute), h.Pair)` inside `routes.go` — move the limiter there since it is pairing-specific, importing it from the `v0` package; if that creates an import cycle, instead pass the limiter as a `gin.HandlerFunc` parameter into `pairing.Register` from `router.go`). Resolve whichever shape compiles cleanly and keep the resulting route list identical: `POST /pair/code` (unguarded, unlimited), `POST /pair` (unguarded, rate-limited), `GET /clients` + `DELETE /clients/:id` (guarded).

Keep the existing `health.Register(rg)` and `system.Register(rg)` calls — do not duplicate `health.Register`; only move it earlier if it was not already before `rejectEmptyPathParams()`. Read the current file to confirm the right placement rather than guessing from this plan's summary.

- [ ] **Step 7: Full-package build and test**

Run: `cd api && go build -tags noEmbed ./... && go test -tags noEmbed ./internal/... -v 2>&1 | tail -100`
Expected: builds cleanly; all tests pass, including every existing test in `api/internal/api/v0` (confirm nothing else broke from the router reordering).

- [ ] **Step 8: Commit**

```bash
git add api/internal/api/v0/ api/internal/app/container.go
git commit -m "feat(api): mount pairing/client-registry routes behind bearer auth"
```

---

### Task 6: `GET /v0/system/browse` — remote directory listing

**Files:**
- Create: `api/internal/api/v0/endpoints/system/handlers/browse.go`
- Create: `api/internal/api/v0/endpoints/system/handlers/browse_test.go`
- Modify: `api/internal/api/v0/endpoints/system/routes.go` (or wherever `system.Register` mounts `prerequisites.go`'s route — read `api/internal/api/v0/endpoints/system/` in full first and mirror its exact registration shape)

**Interfaces:**
- Produces: `GET /v0/system/browse?path=<abs path>` → `[]BrowseEntry{Name, Path, IsDir}`, sorted directories-first then by name. Empty `path` defaults to the daemon process's home directory.

- [ ] **Step 1: Write the failing test**

```go
package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBrowse_ListsDirectory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "zeta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequest(http.MethodGet, "/browse?path="+dir, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Dirs first (alpha, zeta), then files (readme.txt).
	body := rec.Body.String()
	alphaIdx, zetaIdx, readmeIdx := indexOf(body, `"alpha"`), indexOf(body, `"zeta"`), indexOf(body, `"readme.txt"`)
	if !(alphaIdx < zetaIdx && zetaIdx < readmeIdx) {
		t.Fatalf("expected dirs-first, alpha-before-zeta ordering; got body: %s", body)
	}
}

func TestBrowse_NonexistentPath_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequest(http.MethodGet, "/browse?path=/definitely/not/a/real/path/xyz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("expected a 4xx for a nonexistent path, got %d", rec.Code)
	}
}

func TestBrowse_PathIsAFile_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	file := filepath.Join(dir, "notadir.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequest(http.MethodGet, "/browse?path="+file, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("expected a 4xx for a file path, got %d", rec.Code)
	}
}

func TestBrowse_EmptyPath_DefaultsToHome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequest(http.MethodGet, "/browse", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an empty path (home default), got %d: %s", rec.Code, rec.Body.String())
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/endpoints/system/... -run TestBrowse -v`
Expected: FAIL — `Browse` undefined.

- [ ] **Step 3: Implement `browse.go`**

```go
// Package handlers also serves GET /v0/system/browse: a directory listing
// for the remote file-picker client, unrelated to and unsandboxed by any
// workspace (there is no workspace yet — this is how a client picks WHERE to
// create one). Mirror this file's existing handler in the same package for
// the request/response envelope convention (libs.WriteErr/WriteQueryOK).
package handlers

import (
	"net/http"
	"os"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// BrowseEntry is one child of a listed directory.
type BrowseEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// Browse serves GET /v0/system/browse?path=<absolute path>. An empty path
// defaults to the daemon process's home directory. Files are listed but
// inert on the client — see spec §5.3 — so this endpoint returns them too,
// rather than filtering.
func Browse(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			libs.WriteErr(c, http.StatusInternalServerError, "could not resolve a home directory")
			return
		}
		path = home
	}

	info, err := os.Stat(path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "cannot read path: "+err.Error())
		return
	}
	if !info.IsDir() {
		libs.WriteErr(c, http.StatusBadRequest, "path is not a directory")
		return
	}

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "cannot read directory: "+err.Error())
		return
	}

	entries := make([]BrowseEntry, 0, len(dirEntries))
	for _, e := range dirEntries {
		entries = append(entries, BrowseEntry{
			Name:  e.Name(),
			Path:  path + string(os.PathSeparator) + e.Name(),
			IsDir: e.IsDir(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	libs.WriteQueryOK(c, entries)
}
```

- [ ] **Step 4: Mount the route**

Read `api/internal/api/v0/endpoints/system/` in full (routes + handlers files) and add `GET /browse` to whatever registration function `system.Register` already calls, following the exact same pattern `prerequisites.go`'s route uses.

- [ ] **Step 5: Run to verify all pass**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/endpoints/system/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/api/v0/endpoints/system/
git commit -m "feat(api): GET /v0/system/browse — directory listing for the remote file picker"
```

---

### Task 7: server-TLS-only TCP transport + CLI `crowbar pair`

**Files:**
- Modify: `api/internal/core/gateway/transports/tcp.go` (read in full first)
- Modify: `api/internal/core/gateway/transports/tcp_test.go`
- Modify: `api/internal/core/gateway/gateway.go`
- Modify: `api/internal/internal.go` (read in full first — the plan's research already quotes it in full above; re-read before editing since it may have drifted)
- Modify: `api/cmd/crowbar/main.go` (add `newPairCmd()` to the command list)
- Create: `api/cmd/crowbar/client.go`, `api/cmd/crowbar/client_test.go`
- Create: `api/cmd/crowbar/pair.go`, `api/cmd/crowbar/pair_test.go`

**Interfaces:**
- Produces: `transports.NewTCP(host string, id *tlsid.Identity) (net.Listener, error)`, `gateway.New(host string, id *tlsid.Identity) (net.Listener, error)`, CLI command `crowbar pair`.

- [ ] **Step 1: Update `tcp.go`'s failing test for server-TLS-only (no client cert requirement)**

Read the existing `tcp_test.go` in full first. Add/adjust a test asserting the TLS config no longer requires a client certificate:

```go
func TestNewTCP_NonLoopback_ServerTLSOnly_NoClientCertRequired(t *testing.T) {
	dir := t.TempDir()
	id, err := tlsid.Load(dir)
	if err != nil {
		t.Fatalf("tlsid.Load: %v", err)
	}
	ln, err := NewTCP("tcp://0.0.0.0:0", id)
	if err != nil {
		t.Fatalf("NewTCP: %v", err)
	}
	defer ln.Close()

	tlsLn, ok := ln.(interface{ Addr() net.Addr })
	_ = tlsLn
	if !ok {
		t.Fatal("expected a listener")
	}
	// Dial without presenting any client certificate — must succeed the
	// handshake (server-TLS-only, not mTLS).
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // test dials our own self-signed listener
	if err != nil {
		t.Fatalf("expected a client with no cert to complete the TLS handshake, got: %v", err)
	}
	conn.Close()
}
```

Add `"crypto/tls"` and the `tlsid` import to the test file if not already present. Also adjust any EXISTING test in this file that asserted `tls.RequireAnyClientCert`/mTLS behavior — read them and update to assert the new server-TLS-only expectation instead of deleting coverage.

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test -tags noEmbed ./internal/core/gateway/transports/... -v`
Expected: FAIL — `NewTCP` doesn't take an `id` param yet (or, if a stale mTLS version already exists from elsewhere, the handshake-without-cert assertion fails).

- [ ] **Step 3: Implement**

```go
package transports

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

// NewTCP binds a TCP listener. A loopback bind stays plaintext — the dev
// path — regardless of id. A non-loopback bind terminates TLS using id's
// server identity when supplied, and is refused outright when it is not; no
// client certificate is ever requested (server-TLS-only, not mTLS — auth is
// the bearer token carried in the request, not the connection).
func NewTCP(
	host string,
	id *tlsid.Identity,
) (net.Listener, error) {
	addr := strings.TrimPrefix(host, "tcp://")
	if !isNonLoopbackBind(addr) {
		// Loopback is the dev path and stays plaintext: make dev-api and
		// crowbar-seed both dial it over http.
		return net.Listen("tcp", addr)
	}
	if id == nil {
		return nil, fmt.Errorf(
			"gateway: refusing to bind %s without a TLS identity — the API would be "+
				"reachable and unauthenticated", addr)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{id.Certificate},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS13,
	}), nil
}

// isNonLoopbackBind reports whether addr binds beyond loopback. An empty or
// wildcard host (":3737", "0.0.0.0:3737", "[::]:3737") binds all interfaces; a
// concrete loopback host (127.0.0.0/8, ::1, localhost) does not.
func isNonLoopbackBind(addr string) bool {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		h = addr
	}
	switch h {
	case "", "0.0.0.0", "::", "[::]":
		return true
	case "localhost", "::1", "[::1]":
		return false
	}
	if strings.HasPrefix(h, "127.") {
		return false
	}
	if ip := net.ParseIP(h); ip != nil {
		return !ip.IsLoopback()
	}
	return true
}
```

Update `gateway.go`:

```go
package gateway

import (
	"fmt"
	"net"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

func New(host string, id *tlsid.Identity) (net.Listener, error) {
	switch {
	case strings.HasPrefix(host, "unix://"):
		return transports.NewSocket(host)
	case strings.HasPrefix(host, "tcp://"):
		return transports.NewTCP(host, id)
	default:
		return nil, fmt.Errorf("unsupported host scheme %q: use unix:// or tcp://", host)
	}
}
```

Update `internal.go`'s `New` function: before the `gateway.New(host)` call (currently line ~60), resolve the identity directory and load it, then pass it through:

```go
	homeDir := cfg.homeDir
	if homeDir == "" {
		homeDir = metadata.GetHomePath()
	}
	identity, err := tlsid.Load(filepath.Join(homeDir, "tls"))
	if err != nil {
		return nil, fmt.Errorf("internal: tls identity: %w", err)
	}

	listener, err := gateway.New(host, identity)
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}
```

Add `"path/filepath"` and `"github.com/char2cs/crowbar/api/internal/core/tlsid"` to `internal.go`'s imports. Note this now loads a TLS identity even for a plain unix-socket/loopback dev boot — harmless (the file just sits unused on disk) and keeps `New`'s signature simple; do not special-case it away.

Update `main.go`'s `newServeCmd`'s flag help text (currently warns "non-loopback TCP exposes the unauthenticated API to the network" — no longer accurate):

```go
	cmd.Flags().StringVar(&host, "host", "unix://", "listen address (unix:// or tcp://127.0.0.1:3737; non-loopback TCP terminates TLS and requires paired clients)")
```

- [ ] **Step 4: Run to verify transport tests pass, then the whole `api/` package builds**

Run: `cd api && go test -tags noEmbed ./internal/core/gateway/... -v && go build -tags noEmbed ./...`
Expected: PASS, clean build.

- [ ] **Step 5: Write the failing CLI tests**

```go
// client_test.go
package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGetJSON_DecodesSuccessEnvelope(t *testing.T) {
	socket := unixTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	socket := unixTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope[map[string]string]{Success: false, Error: "nope"})
	})
	if _, err := postJSON[map[string]string](socket, "/x"); err == nil {
		t.Fatal("expected an error for a refused envelope")
	}
}

// unixTestServer starts an httptest server over a temp unix socket and
// returns the socket path.
func unixTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	dir := t.TempDir()
	socket := filepath.Join(dir, "test.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close(); _ = os.Remove(socket) })
	return socket
}
```

```go
// pair_test.go
package main

import (
	"bytes"
	"testing"
)

func TestRunPair_PrintsCodeAndFingerprint(t *testing.T) {
	socket := unixTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope[mintCodeResponse]{Success: true, Data: mintCodeResponse{Code: "123456"}})
	})
	var out bytes.Buffer
	if err := runPair(socket, &out); err != nil {
		t.Fatalf("runPair: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("123456")) {
		t.Fatalf("expected the code in output, got: %s", out.String())
	}
}
```

Add `"encoding/json"`, `"net/http"` to `pair_test.go`'s imports.

- [ ] **Step 6: Run to verify failure, then implement `client.go` and `pair.go`**

```go
// client.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// localClient returns an HTTP client that reaches the local daemon over its
// unix socket.
func localClient(socket string) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}
}

// envelope is the {success,data,error} shape every /v0 route answers with.
type envelope[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Error   string `json:"error"`
}

func getJSON[T any](socket, path string) (T, error) {
	var zero T
	resp, err := localClient(socket).Get("http://crowbar" + path)
	if err != nil {
		return zero, fmt.Errorf("cannot reach the crowbar daemon: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeEnvelope[T](resp)
}

func postJSON[T any](socket, path string) (T, error) {
	var zero T
	resp, err := localClient(socket).Post("http://crowbar"+path, "application/json", nil)
	if err != nil {
		return zero, fmt.Errorf("cannot reach the crowbar daemon: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeEnvelope[T](resp)
}

func decodeEnvelope[T any](resp *http.Response) (T, error) {
	var zero T
	var env envelope[T]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return zero, fmt.Errorf("daemon returned an unreadable response: %w", err)
	}
	if !env.Success {
		return zero, fmt.Errorf("daemon refused: %s", env.Error)
	}
	return env.Data, nil
}
```

```go
// pair.go
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

func newPairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pair",
		Short: "Mint a one-time pairing code for connecting a Crowbar client",
		RunE: func(_ *cobra.Command, _ []string) error {
			socket, err := transports.SocketPath("unix://")
			if err != nil {
				return err
			}
			return runPair(socket, os.Stdout)
		},
	}
}

type mintCodeResponse struct {
	Code string `json:"code"`
}

// runPair mints a pairing code and prints it for the operator to hand to a
// client, alongside its expiry.
func runPair(socket string, out io.Writer) error {
	got, err := postJSON[mintCodeResponse](socket, "/v0/pair/code")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, `
Pairing code    %s

Enter it in the Crowbar client within %s. It works once.

`, got.Code, pairing.CodeTTL)
	return nil
}
```

Confirm `transports.SocketPath` still exists with this exact signature (it was used unchanged by the backed-up branch's own `doctor.go`, which is new-to-this-feature, but `SocketPath` itself is likely a pre-existing helper — grep for it before assuming).

- [ ] **Step 7: Register the command and run the full CLI test suite**

In `main.go`'s `newRootCmd`, add `newPairCmd()` to the `root.AddCommand(...)` call.

Run: `cd api && go test -tags noEmbed ./cmd/crowbar/... -v && go build -tags noEmbed ./...`
Expected: PASS, clean build.

- [ ] **Step 8: Commit**

```bash
git add api/internal/core/gateway/ api/internal.go api/internal/internal.go api/cmd/crowbar/main.go api/cmd/crowbar/client.go api/cmd/crowbar/client_test.go api/cmd/crowbar/pair.go api/cmd/crowbar/pair_test.go
git commit -m "feat(api): server-TLS-only TCP transport, wire the daemon's identity, crowbar pair CLI"
```

---

### Task 8: full-branch gate

**Files:** none new — this task runs the project's own gates and fixes anything Tasks 1–7 broke elsewhere.

- [ ] **Step 1: Run the full Go gate**

Run: `cd api && go build -tags noEmbed ./... && go test -tags noEmbed ./... 2>&1 | tail -150`
Expected: builds cleanly, all tests pass (including every pre-existing test untouched by this plan — the router reordering in Task 5 is the most likely place for a regression: confirm `api/internal/api/v0/route_audit_test.go` still passes, since it likely asserts something about the full route table).

- [ ] **Step 2: Run lint and deadcode gates**

Run: `cd api && make lint 2>&1 | tail -80` and `cd api && make deadcode 2>&1 | tail -40`
Expected: clean on new code (existing `lint-backlog` items are not this plan's concern).

- [ ] **Step 3: Fix anything the gates surface, then commit**

If fixes are needed, make them and commit with a message describing what the gate caught — do not bundle unrelated cleanup.
