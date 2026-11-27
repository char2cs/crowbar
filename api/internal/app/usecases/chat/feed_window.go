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
	previewBytes     = 8 << 10
	DefaultPageBytes = 256 << 10
	MaxPageBytes     = 4 << 20
)

func previewCapOf(windowBytes int64) int64 {
	if windowBytes > 0 {
		return previewBytes
	}
	return 0
}

type RowsPage struct {
	Rows []ledger.Row `json:"rows"`
	From int64        `json:"from"`
}

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

func (u *Usecase) knownChat(ctx context.Context, op, chatID string) error {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		if errors.Is(err, agentchat.ErrNotFound) {
			return fmt.Errorf("%s: %s: %w: %w", op, chatID, err, problem.ErrNotFound)
		}
		return fmt.Errorf("%s: %s: %w", op, chatID, err)
	}
	return nil
}
