package main

import (
	"log"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallLogRing_DefaultLoggerAndStdlibLog_FeedTheRing(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	ring := installLogRing()
	slog.Info("via slog", "component", "t")
	slog.Debug("below the default level")
	log.Print("via stdlib")

	stream := ring.Stream(0, slog.LevelDebug)
	defer stream.Close()
	replay := stream.Replay()
	require.Len(t, replay, 2)
	assert.Equal(t, "via slog", replay[0].Msg)
	assert.Equal(t, "t", replay[0].Component)
	assert.Equal(t, "via stdlib", replay[1].Msg)
}
