package sqlite

// Internal tests for primaryKeyColumn's schema-parse error branch, which is
// unreachable from the external test package without direct access to the
// unexported helper.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unparseableSchema has a function-typed field, which GORM's schema parser
// rejects with "unsupported data type", exercising primaryKeyColumn's
// stmt.Parse error branch directly.
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
