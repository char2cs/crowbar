package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// RowReader reads a chat's older rows by wire size, and one row whole.
type RowReader interface {
	ReadRows(ctx context.Context, chatID string, before, bytes int64) (agentusecase.RowsPage, error)
	ReadRow(ctx context.Context, chatID string, seq int64) (ledger.Row, error)
}

// LedgerRows serves GET /v0/chats/:chatId/rows and /rows/:seq.
type LedgerRows struct{ rows RowReader }

// NewLedgerRows builds the rows handler over rows.
func NewLedgerRows(rows RowReader) *LedgerRows { return &LedgerRows{rows: rows} }

// rowsPage is the answer to an older page: More says rows older than From exist.
type rowsPage struct {
	Rows []ledger.Row `json:"rows"`
	From int64        `json:"from"`
	More bool         `json:"more"`
}

// Page answers the whole turns before ?before=<seq>, newest within ?bytes=<n>.
func (h *LedgerRows) Page(c *gin.Context) {
	before, err := queryInt(c, "before", 0)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	bytes, err := queryInt(c, "bytes", agentusecase.DefaultPageBytes)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	page, err := h.rows.ReadRows(c.Request.Context(), c.Param("chatId"), before, bytes)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, rowsPage{Rows: page.Rows, From: page.From, More: page.From > 0})
}

// One answers row :seq whole, as a preview of it left it out.
func (h *LedgerRows) One(c *gin.Context) {
	seq, err := strconv.ParseInt(c.Param("seq"), 10, 64)
	if err != nil {
		libs.Fail(c, fmt.Errorf("row seq %q: %w", c.Param("seq"), problem.ErrInvalidArgument))
		return
	}
	row, err := h.rows.ReadRow(c.Request.Context(), c.Param("chatId"), seq)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, row)
}

// queryInt is the integer query parameter name, def when absent.
func queryInt(c *gin.Context, name string, def int64) (int64, error) {
	raw := c.Query(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("query %s %q: %w", name, raw, problem.ErrInvalidArgument)
	}
	return n, nil
}
