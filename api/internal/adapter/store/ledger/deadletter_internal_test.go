package ledger

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

func TestStore_DeadLetterCountReadsAnIndex(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	_, err = Open(t.Context(), db, time.Now)
	require.NoError(t, err)

	var plan []struct{ Detail string }
	require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+deadLetterCountSQL, []string{"a", "b"}).Scan(&plan).Error)

	require.NotEmpty(t, plan)
	assert.Contains(t, fmt.Sprint(plan), "COVERING INDEX")
}
