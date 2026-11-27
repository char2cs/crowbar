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

type RowReader interface {
	ReadRows(ctx context.Context, chatID string, before, bytes int64) (agentusecase.RowsPage, error)
	ReadRow(ctx context.Context, chatID string, seq int64) (ledger.Row, error)
}

type LedgerRows struct{ rows RowReader }

func NewLedgerRows(rows RowReader) *LedgerRows { return &LedgerRows{rows: rows} }

type rowsPage struct {
	Rows []ledger.Row `json:"rows"`
	From int64        `json:"from"`
	More bool         `json:"more"`
}

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
