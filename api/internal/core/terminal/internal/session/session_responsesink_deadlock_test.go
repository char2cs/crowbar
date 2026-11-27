//go:build !windows

package session

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

func TestModelDriven_ResponseSinkNeverWedgesTeardown(t *testing.T) {
	s := newBareSession("sid-md-sink-deadlock", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	s.ptmx = pw
	t.Cleanup(func() { _ = pr.Close() })

	s.startResponseSink(s.ptmx)

	burst := bytes.Repeat([]byte("\x1b[6n"), 12000)

	pumped := make(chan struct{})
	go func() {
		s.PumpChunkForTest(burst)
		close(pumped)
	}()

	<-pumped

	killed := make(chan struct{})
	go func() {
		s.Kill()
		close(killed)
	}()

	<-killed

	<-pumped
}
