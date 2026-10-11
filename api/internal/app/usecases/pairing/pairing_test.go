package pairing

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
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

// TestRedeem_TokenIs32RandomBytes pins the bearer token's strength: 64 hex
// characters (32 bytes from crypto/rand), never repeated across redeems.
func TestRedeem_TokenIs32RandomBytes(t *testing.T) {
	u := New(&fakeClientStore{})
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		now := time.Now()
		code, err := u.Mint(now)
		if err != nil {
			t.Fatalf("Mint: %v", err)
		}
		token, _, err := u.Redeem(context.Background(), code, "x", now)
		if err != nil {
			t.Fatalf("Redeem: %v", err)
		}
		raw, err := hex.DecodeString(token)
		if err != nil || len(raw) != 32 {
			t.Fatalf("expected 32 hex-encoded bytes, got %q", token)
		}
		if seen[token] {
			t.Fatalf("token %q was issued twice", token)
		}
		seen[token] = true
	}
}

// TestRedeem_WrongGuessesBurnTheCode proves maxFailedRedeems wrong guesses,
// however they are spread across callers, burn the outstanding code: the right
// code no longer redeems, so a brute force gets maxFailedRedeems tries per code.
func TestRedeem_WrongGuessesBurnTheCode(t *testing.T) {
	u := New(&fakeClientStore{})
	now := time.Now()
	code, err := u.Mint(now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	wrong := wrongCode(code)
	for i := 0; i < maxFailedRedeems; i++ {
		if _, _, err := u.Redeem(context.Background(), wrong, "x", now); !errors.Is(err, ErrBadCode) {
			t.Fatalf("guess %d: expected ErrBadCode, got %v", i, err)
		}
	}
	if _, _, err := u.Redeem(context.Background(), code, "x", now); !errors.Is(err, ErrNoCode) {
		t.Fatalf("expected the burned code to be gone, got %v", err)
	}
}

// TestRedeem_FewerWrongGuessesLeaveTheCode proves a guess short of the burn
// threshold does not lock the operator out, and a fresh mint resets the count.
func TestRedeem_FewerWrongGuessesLeaveTheCode(t *testing.T) {
	u := New(&fakeClientStore{})
	now := time.Now()
	first, err := u.Mint(now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	for i := 0; i < maxFailedRedeems-1; i++ {
		_, _, _ = u.Redeem(context.Background(), wrongCode(first), "x", now)
	}
	second, err := u.Mint(now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	for i := 0; i < maxFailedRedeems-1; i++ {
		_, _, _ = u.Redeem(context.Background(), wrongCode(second), "x", now)
	}
	if _, _, err := u.Redeem(context.Background(), second, "x", now); err != nil {
		t.Fatalf("expected the code to survive %d wrong guesses after a fresh mint, got %v", maxFailedRedeems-1, err)
	}
}

func wrongCode(code string) string {
	if code == "000000" {
		return "000001"
	}
	return "000000"
}

func TestHashToken_IsStableAndDistinct(t *testing.T) {
	h1, h2 := HashToken("a"), HashToken("a")
	if h1 != h2 {
		t.Fatal("HashToken must be deterministic")
	}
	if HashToken("a") == HashToken("b") {
		t.Fatal("HashToken must distinguish different inputs")
	}
}

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
