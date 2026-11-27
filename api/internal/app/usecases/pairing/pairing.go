package pairing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
)

var (
	ErrNoCode      = errors.New("pairing: no code outstanding")
	ErrBadCode     = errors.New("pairing: code does not match")
	ErrCodeExpired = errors.New("pairing: code expired")
)

const CodeTTL = 5 * time.Minute

const maxFailedRedeems = 10

type ClientStore interface {
	Save(ctx context.Context, c domain.Client) error
}

type Usecase interface {
	Mint(now time.Time) (code string, err error)
	Redeem(ctx context.Context, code, name string, now time.Time) (token string, client domain.Client, err error)
}

type usecase struct {
	clients ClientStore

	mu       sync.Mutex
	code     string
	expires  time.Time
	failures int
}

func New(clients ClientStore) Usecase {
	return &usecase{clients: clients}
}

func (u *usecase) Mint(now time.Time) (string, error) {
	code, err := newCode()
	if err != nil {
		return "", err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.code = code
	u.expires = now.Add(CodeTTL)
	u.failures = 0
	return code, nil
}

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

func (u *usecase) claim(code string, now time.Time) (string, time.Time, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.code == "" {
		return "", time.Time{}, ErrNoCode
	}
	if subtle.ConstantTimeCompare([]byte(u.code), []byte(code)) != 1 {
		u.failures++
		if u.failures >= maxFailedRedeems {
			u.code, u.expires = "", time.Time{}
			slog.Warn("pairing: code burned after repeated wrong guesses; mint a new one with `crowbar pair`")
		}
		return "", time.Time{}, ErrBadCode
	}
	if now.After(u.expires) {
		return "", time.Time{}, ErrCodeExpired
	}

	claimed, claimedExp := u.code, u.expires
	u.code, u.expires = "", time.Time{}
	return claimed, claimedExp, nil
}

func (u *usecase) restore(code string, expires time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.code == "" {
		u.code, u.expires = code, expires
	}
}

func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("pairing: entropy: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("pairing: token entropy: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
