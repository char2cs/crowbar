package branchchat_test

import (
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/branchchat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var epoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// TestFrom_EmptyInput ensures an empty slice returns an empty (non-nil) slice.
func TestFrom_EmptyInput(t *testing.T) {
	t.Parallel()

	got := branchchat.From(nil, epoch)

	if got == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("expected len 0, got %d", len(got))
	}
}

// TestFrom_NoActiveIndicator verifies that From sets no active/agent-running
// indicator: the IsActive field was removed along with the agent-running status
// producer (D11), so every projection is a plain ID/Title/Age triple regardless
// of chat status.
func TestFrom_NoActiveIndicator(t *testing.T) {
	t.Parallel()

	chats := []domain.Chat{
		{
			ID:        "a",
			Title:     "Chat A",
			Status:    domain.ChatStatusIdle,
			CreatedAt: epoch,
		},
		{
			ID:        "b",
			Title:     "Chat B",
			Status:    domain.ChatStatusIdle,
			CreatedAt: epoch,
		},
	}

	got := branchchat.From(chats, epoch)

	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	want := []domain.BranchChat{
		{ID: "a", Title: "Chat A", Age: "0s"},
		{ID: "b", Title: "Chat B", Age: "0s"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestFrom_TitleAndIDPassThrough verifies ID and Title are copied.
func TestFrom_TitleAndIDPassThrough(t *testing.T) {
	t.Parallel()

	chats := []domain.Chat{
		{
			ID:        "chat-123",
			Title:     "My Branch Discussion",
			Status:    domain.ChatStatusIdle,
			CreatedAt: epoch,
		},
	}

	got := branchchat.From(chats, epoch)

	if got[0].ID != "chat-123" {
		t.Errorf("expected ID chat-123, got %q", got[0].ID)
	}
	if got[0].Title != "My Branch Discussion" {
		t.Errorf("expected Title 'My Branch Discussion', got %q", got[0].Title)
	}
}

// TestFrom_AgeNonEmpty ensures Age is populated for any chat.
func TestFrom_AgeNonEmpty(t *testing.T) {
	t.Parallel()

	now := epoch
	chats := []domain.Chat{
		{
			ID:        "c",
			Status:    domain.ChatStatusIdle,
			CreatedAt: now.Add(-5 * time.Minute),
		},
	}

	got := branchchat.From(chats, now)

	if got[0].Age == "" {
		t.Error("expected non-empty Age")
	}
}

// TestFrom_AgeBucketSeconds verifies the "Xs" bucket for sub-minute age.
func TestFrom_AgeBucketSeconds(t *testing.T) {
	t.Parallel()

	now := epoch
	chats := []domain.Chat{
		{ID: "s", Status: domain.ChatStatusIdle, CreatedAt: now.Add(-30 * time.Second)},
	}

	got := branchchat.From(chats, now)

	if got[0].Age != "30s" {
		t.Errorf("expected '30s', got %q", got[0].Age)
	}
}

// TestFrom_AgeBucketMinutes verifies the "Xm" bucket for sub-hour age.
func TestFrom_AgeBucketMinutes(t *testing.T) {
	t.Parallel()

	now := epoch
	chats := []domain.Chat{
		{ID: "m", Status: domain.ChatStatusIdle, CreatedAt: now.Add(-45 * time.Minute)},
	}

	got := branchchat.From(chats, now)

	if got[0].Age != "45m" {
		t.Errorf("expected '45m', got %q", got[0].Age)
	}
}

// TestFrom_AgeBucketHours verifies the "Xh" bucket for sub-day age.
func TestFrom_AgeBucketHours(t *testing.T) {
	t.Parallel()

	now := epoch
	chats := []domain.Chat{
		{ID: "h", Status: domain.ChatStatusIdle, CreatedAt: now.Add(-3 * time.Hour)},
	}

	got := branchchat.From(chats, now)

	if got[0].Age != "3h" {
		t.Errorf("expected '3h', got %q", got[0].Age)
	}
}

// TestFrom_AgeBucketDays verifies the "Xd" bucket for age >= 1 day.
func TestFrom_AgeBucketDays(t *testing.T) {
	t.Parallel()

	now := epoch
	chats := []domain.Chat{
		{ID: "d", Status: domain.ChatStatusIdle, CreatedAt: now.Add(-48 * time.Hour)},
	}

	got := branchchat.From(chats, now)

	if got[0].Age != "2d" {
		t.Errorf("expected '2d', got %q", got[0].Age)
	}
}

// TestFrom_AgeFutureCreatedAt verifies that a chat created in the future
// (clock skew) does not produce a negative age — it returns "0s".
func TestFrom_AgeFutureCreatedAt(t *testing.T) {
	t.Parallel()

	now := epoch
	chats := []domain.Chat{
		{ID: "f", Status: domain.ChatStatusIdle, CreatedAt: now.Add(10 * time.Second)},
	}

	got := branchchat.From(chats, now)

	if got[0].Age != "0s" {
		t.Errorf("expected '0s' for future createdAt, got %q", got[0].Age)
	}
}
