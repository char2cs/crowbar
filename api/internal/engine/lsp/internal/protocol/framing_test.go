package protocol

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFraming_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte(`{"jsonrpc":"2.0","id":1}`)

	require.NoError(t, WriteMessage(&buf, payload))

	r := bufio.NewReader(&buf)
	got, err := ReadMessage(r)
	require.NoError(t, err)
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1}`, string(got))
}

func TestFraming_TwoMessages(t *testing.T) {
	var buf bytes.Buffer

	first := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	second := []byte(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`)

	require.NoError(t, WriteMessage(&buf, first))
	require.NoError(t, WriteMessage(&buf, second))

	r := bufio.NewReader(&buf)

	got1, err := ReadMessage(r)
	require.NoError(t, err)
	assert.JSONEq(t, string(first), string(got1))

	got2, err := ReadMessage(r)
	require.NoError(t, err)
	assert.JSONEq(t, string(second), string(got2))
}

func TestFraming_BadHeaderErrors(t *testing.T) {
	// Garbage header line — no ":" separator.
	r := bufio.NewReader(strings.NewReader("garbage\r\n\r\n{}"))
	_, err := ReadMessage(r)
	require.Error(t, err)
}

func TestFraming_MissingContentLength(t *testing.T) {
	// Valid header format but without Content-Length.
	r := bufio.NewReader(strings.NewReader("Content-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n{}"))
	_, err := ReadMessage(r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Content-Length")
}

func TestFraming_NonNumericContentLength(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("Content-Length: abc\r\n\r\n{}"))
	_, err := ReadMessage(r)
	require.Error(t, err)
}

func TestFraming_NegativeContentLength(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("Content-Length: -1\r\n\r\n{}"))
	_, err := ReadMessage(r)
	require.Error(t, err)
}

func TestFraming_TruncatedBody(t *testing.T) {
	// Content-Length says 100 bytes but body is only 2 bytes.
	r := bufio.NewReader(strings.NewReader("Content-Length: 100\r\n\r\n{}"))
	_, err := ReadMessage(r)
	require.Error(t, err)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestFraming_EmptyPayload(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteMessage(&buf, []byte{}))

	r := bufio.NewReader(&buf)
	got, err := ReadMessage(r)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFraming_WriteError(t *testing.T) {
	// errWriter always fails.
	err := WriteMessage(&errWriter{}, []byte(`{}`))
	require.Error(t, err)
}

func TestFraming_EOFOnFirstRead(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(""))
	_, err := ReadMessage(r)
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestFraming_HeaderWithExtraFields(t *testing.T) {
	// Real LSP servers send Content-Type alongside Content-Length; we should
	// ignore unknown headers and still succeed.
	const msg = "Content-Length: 2\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n{}"
	r := bufio.NewReader(strings.NewReader(msg))
	got, err := ReadMessage(r)
	require.NoError(t, err)
	assert.Equal(t, []byte("{}"), got)
}

// errWriter always returns an error on Write.
type errWriter struct{}

func (e *errWriter) Write(
	_ []byte,
) (int, error) {
	return 0, fmt.Errorf("write error")
}
