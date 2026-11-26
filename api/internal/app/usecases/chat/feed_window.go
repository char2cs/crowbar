package chat

import (
	"context"
	"errors"
	"fmt"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

const (
	// previewBytes is the most of a bulky body field a replay sends of a row.
	previewBytes = 8 << 10
	// DefaultPageBytes is the wire size of an older page when its request names none.
	DefaultPageBytes = 256 << 10
	// MaxPageBytes is the most one older page may ask for.
	MaxPageBytes = 4 << 20
)

// previewCapOf is the size a window counts a previewable row at: its preview's,
// when the window is of bytes and so sent as previews.
func previewCapOf(windowBytes int64) int64 {
	if windowBytes > 0 {
		return previewBytes
	}
	return 0
}

// RowsPage is a run of whole turns older than a row a client holds. From is the
// seq its first row follows: 0 when nothing is older.
type RowsPage struct {
	Rows []ledger.Row `json:"rows"`
	From int64        `json:"from"`
}

// replay visits chatID's rows after `after` up to upto, as previews when asked
// to; it stops, with the context's error, when visit says so.
func (u *Usecase) replay(
	ctx context.Context, chatID string, after, upto int64, preview bool, visit func(ledger.Row) bool,
) error {
	for after < upto {
		rows, err := u.ledger.Read(ctx, chatID, after, int(min(upto-after, feedReplayPage)))
		if err != nil {
			return fmt.Errorf("chat feed: %s: replay: %w", chatID, err)
		}
		if len(rows) == 0 {
			return nil
		}
		for _, r := range rows {
			if preview {
				r = ledger.Preview(r, previewBytes)
			}
			if !visit(r) {
				return ctx.Err()
			}
		}
		after = rows[len(rows)-1].Seq
	}
	return nil
}

// ReadRows is the whole turns right before row `before` (the From a window or
// page gave), newest within bytes of wire size, as previews. An unknown chat is
// problem.ErrNotFound, a before below 1 problem.ErrInvalidArgument.
func (u *Usecase) ReadRows(ctx context.Context, chatID string, before, bytes int64) (RowsPage, error) {
	if before < 1 {
		return RowsPage{}, fmt.Errorf("chat rows: before %d: %w", before, problem.ErrInvalidArgument)
	}
	if err := u.knownChat(ctx, "chat rows", chatID); err != nil {
		return RowsPage{}, err
	}
	bytes = min(max(bytes, 1), MaxPageBytes)
	from, err := u.ledger.Window(ctx, chatID, ledgerstore.WindowSpec{Bytes: bytes, Upto: before, PreviewCap: previewBytes}, nil)
	if err != nil {
		return RowsPage{}, fmt.Errorf("chat rows: %s: %w", chatID, err)
	}
	page := RowsPage{Rows: []ledger.Row{}, From: from}
	err = u.replay(ctx, chatID, from, before, true, func(r ledger.Row) bool {
		page.Rows = append(page.Rows, r)
		return true
	})
	return page, err
}

// ReadRow is row seq of chatID whole, as a preview of it left it out: an
// unknown chat or a seq no row has is problem.ErrNotFound.
func (u *Usecase) ReadRow(ctx context.Context, chatID string, seq int64) (ledger.Row, error) {
	if err := u.knownChat(ctx, "chat row", chatID); err != nil {
		return ledger.Row{}, err
	}
	rows, err := u.ledger.Read(ctx, chatID, seq-1, 1)
	if err != nil {
		return ledger.Row{}, fmt.Errorf("chat row: %s: %w", chatID, err)
	}
	if seq < 1 || len(rows) == 0 || rows[0].Seq != seq {
		return ledger.Row{}, fmt.Errorf("chat row: %s: seq %d: %w", chatID, seq, problem.ErrNotFound)
	}
	return rows[0], nil
}

// knownChat is nil for a chat that exists, else problem.ErrNotFound; op names the caller.
func (u *Usecase) knownChat(ctx context.Context, op, chatID string) error {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		if errors.Is(err, agentchat.ErrNotFound) {
			return fmt.Errorf("%s: %s: %w: %w", op, chatID, err, problem.ErrNotFound)
		}
		return fmt.Errorf("%s: %s: %w", op, chatID, err)
	}
	return nil
}
