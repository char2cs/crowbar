package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unparseableSchema struct {
	F func()
}

func TestPrimaryKeyColumn_ParseError(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)

	_, err = primaryKeyColumn[unparseableSchema](db)
	require.Error(t, err)
}

func TestPrimaryKeyColumn_NoPrimaryKey(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)

	type noPK struct {
		Name string
	}
	_, err = primaryKeyColumn[noPK](db)
	require.Error(t, err)
	assert.Equal(t, "no primary key field found", err.Error())
}

func TestOpenDB_StillSingleConnection(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	assert.Equal(t, 1, sqlDB.Stats().MaxOpenConnections)
}
