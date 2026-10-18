package adapter

import (
	"path/filepath"

	gormdb "gorm.io/gorm"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
)

// ChatLedgerDB is the chat ledger's database under the store directory. It is
// a single connection: the ledger is append-only and has one writer per chat.
func (c *Container) ChatLedgerDB() *gormdb.DB {
	return c.chatLedgerDB
}

func (o *opens) chatLedgerDB(h home.Home) *gormdb.DB {
	return openOne(o, "chat ledger db", func() (*gormdb.DB, error) {
		return storesqlite.OpenDB(filepath.Join(h.Store(), ledgerstore.FileName))
	}, closeViewDB)
}
