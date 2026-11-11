package serialize

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnreadable_WarnsOncePerRow(t *testing.T) {
	var out bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	var u Unreadable

	u.Skip(t.Context(), "chat", "a", errors.New("bad"))
	u.Skip(t.Context(), "chat", "a", errors.New("bad"))
	u.Skip(t.Context(), "chat", "b", errors.New("bad"))

	assert.Equal(t, 1, strings.Count(out.String(), "id=a"))
	assert.Equal(t, 1, strings.Count(out.String(), "id=b"))
}
