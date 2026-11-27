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

type Composer struct {
	Prefix    string
	Draft     string
	Ghost     string
	Busy      bool
	CaretHome bool
}

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

func (l Launch) AwaitSubmitted(tb testing.TB, n int) []string {
	tb.Helper()
	var got []string
	require.Eventually(tb, func() bool {
		got = l.Submitted(tb)
		return len(got) >= n
	}, 10*time.Second, 10*time.Millisecond, "kit: fake composer for runner %s never took %d message(s)", l.RunnerID, n)
	return got
}
