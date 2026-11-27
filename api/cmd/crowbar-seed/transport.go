package main

import (
	"context"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

const tcpScheme = "tcp://"

type transport interface {
	Get(
		ctx context.Context,
		path string,
	) (int, []byte, error)
	PostJSON(
		ctx context.Context,
		path string,
		body any,
	) (int, []byte, error)
	PatchJSON(
		ctx context.Context,
		path string,
		body any,
	) (int, []byte, error)
}

func newTransport(
	host string,
) (transport, error) {
	if strings.HasPrefix(host, tcpScheme) {
		return newTCPTransport(host), nil
	}
	return ipc.NewClient(host)
}
