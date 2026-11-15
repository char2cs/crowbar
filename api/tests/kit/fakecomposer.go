//go:build integration

package kit

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Composer turns the fake CLI into a TUI input box: bracketed paste on, a prompt row, and a submit
// key that records what was typed. The zero value is a plain "> " composer.
type Composer struct {
	Prefix string // text before the cursor on the composer row; "> " when empty
	Draft  string // text already in the box at start, so the cursor sits past the prefix
	Ghost  string // dim suggestion painted after the cursor, which stays right after the prefix
	Busy   bool   // paints no composer and discards input, as a TUI does mid-turn
	// CaretHome leaves the cursor right after the prefix, before the draft, as Home does.
	CaretHome bool
}

// composerEnv is the env the fake CLI reads its composer mode from.
func composerEnv(c *Composer) [][2]string {
	if c == nil {
		return nil
	}
	env := [][2]string{{"FAKECLI_COMPOSER", "1"}}
	for _, kv := range [][2]string{
		{"FAKECLI_COMPOSER_PREFIX", c.Prefix},
		{"FAKECLI_COMPOSER_DRAFT", c.Draft},
		{"FAKECLI_COMPOSER_GHOST", c.Ghost},
	} {
		if kv[1] != "" {
			env = append(env, kv)
		}
	}
	if c.Busy {
		env = append(env, [2]string{"FAKECLI_COMPOSER_BUSY", "1"})
	}
	if c.CaretHome {
		env = append(env, [2]string{"FAKECLI_COMPOSER_CARET_HOME", "1"})
	}
	return env
}

// Submitted is every message the launch's composer took, oldest first: one entry per submit key,
// a pasted message with its newlines intact.
func (l Launch) Submitted(tb testing.TB) []string {
	tb.Helper()
	f, err := os.Open(filepath.Join(l.dir, l.RunnerID+".submitted"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	require.NoError(tb, err)
	defer func() { require.NoError(tb, f.Close()) }()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 16<<20)
	for sc.Scan() {
		var msg string
		require.NoError(tb, json.Unmarshal(sc.Bytes(), &msg))
		out = append(out, msg)
	}
	require.NoError(tb, sc.Err())
	return out
}

// AwaitSubmitted blocks until the composer has taken at least n messages and returns all of them.
func (l Launch) AwaitSubmitted(tb testing.TB, n int) []string {
	tb.Helper()
	var got []string
	require.Eventually(tb, func() bool {
		got = l.Submitted(tb)
		return len(got) >= n
	}, 10*time.Second, 10*time.Millisecond, "kit: fake composer for runner %s never took %d message(s)", l.RunnerID, n)
	return got
}
