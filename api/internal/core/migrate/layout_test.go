package migrate

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestReadLayout(t *testing.T) {
	t.Run("absent file is the legacy layout", func(t *testing.T) {
		got, err := readLayout(testutil.FixtureHome(t))

		require.NoError(t, err)
		assert.Equal(t, 0, got)
	})

	valid := []struct {
		name    string
		content string
		want    int
	}{
		{name: "zero", content: "0\n", want: 0},
		{name: "one", content: "1\n", want: 1},
		{name: "no trailing newline", content: "7", want: 7},
		{name: "surrounding whitespace", content: " 2 \n", want: 2},
		{name: "leading zeros", content: "007\n", want: 7},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			h := testutil.FixtureHome(t)
			testutil.WriteFile(t, h, "LAYOUT", tc.content)

			got, err := readLayout(h)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	garbage := []struct {
		name    string
		content string
	}{
		{name: "empty", content: ""},
		{name: "blank", content: " \n"},
		{name: "letters", content: "abc"},
		{name: "negative", content: "-1"},
		{name: "explicit plus", content: "+1"},
		{name: "fraction", content: "1.5"},
		{name: "two numbers", content: "1 2"},
		{name: "two lines", content: "1\n2\n"},
		{name: "beyond int", content: "99999999999999999999999999"},
	}
	for _, tc := range garbage {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			h := testutil.FixtureHome(t)
			testutil.WriteFile(t, h, "LAYOUT", tc.content)

			_, err := readLayout(h)

			require.ErrorIs(t, err, ErrLayoutUnreadable)
		})
	}
}

func TestReadLayout_ALayoutThatCannotBeReadIsUnreadable(t *testing.T) {
	h := testutil.FixtureHome(t)
	require.NoError(t, os.Mkdir(h.LayoutFile(), 0o700))

	_, err := readLayout(h)

	require.ErrorIs(t, err, ErrLayoutUnreadable)
}

func TestWriteLayout_RoundTripsAtomically(t *testing.T) {
	h := testutil.FixtureHome(t)

	require.NoError(t, writeLayout(h, 3))

	got, err := readLayout(h)
	require.NoError(t, err)
	assert.Equal(t, 3, got)
	raw, err := os.ReadFile(h.LayoutFile())
	require.NoError(t, err)
	assert.Equal(t, "3\n", string(raw))
	assert.Equal(t, []string{"LAYOUT 2"}, testutil.TreeSnapshot(t, h))

	require.NoError(t, writeLayout(h, 12))

	got, err = readLayout(h)
	require.NoError(t, err)
	assert.Equal(t, 12, got)
	assert.Equal(t, []string{"LAYOUT 3"}, testutil.TreeSnapshot(t, h))
}

func TestWriteLayout_ReplacesALeftoverTemp(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "LAYOUT.tmp", "left behind by a crash, much longer than the layout")

	require.NoError(t, writeLayout(h, 1))

	assert.Equal(t, []string{"LAYOUT 2"}, testutil.TreeSnapshot(t, h))
}

func TestWriteLayout_LeavesNoTempWhenTheCommitFails(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "LAYOUT/occupant", "a directory cannot be replaced by a file")

	err := writeLayout(h, 1)

	require.ErrorContains(t, err, "migrate:")
	assert.Equal(t, []string{"LAYOUT/occupant 40"}, testutil.TreeSnapshot(t, h))
}

func TestWriteLayout_RefusesANegativeLayout(t *testing.T) {
	h := testutil.FixtureHome(t)

	err := writeLayout(h, -1)

	require.ErrorContains(t, err, "migrate:")
	assert.Empty(t, testutil.TreeSnapshot(t, h))
}

func TestWriteLayout_FailsWhenTheHomeDoesNotExist(t *testing.T) {
	h := testutil.FixtureHome(t)
	require.NoError(t, os.Remove(h.Root()))

	err := writeLayout(h, 1)

	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "migrate:")
	assert.NoDirExists(t, h.Root())
}
