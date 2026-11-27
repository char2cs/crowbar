package adapter

import (
	"path/filepath"

	gormdb "gorm.io/gorm"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
)

func (c *Container) ChatLedgerDB() *gormdb.DB {
	return c.chatLedgerDB
}

func (o *opens) chatLedgerDB(h home.Home) *gormdb.DB {
	return openOne(o, "chat ledger db", func() (*gormdb.DB, error) {
		return storesqlite.OpenDB(filepath.Join(h.Store(), ledgerstore.FileName))
	}, closeViewDB)
}
