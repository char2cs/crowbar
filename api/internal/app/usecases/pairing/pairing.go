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
