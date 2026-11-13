package connect_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/connect"
)

func execReturning(out string, err error) connect.Exec {
	return func(context.Context, []string, []string) ([]byte, error) { return []byte(out), err }
}

func TestResolve(t *testing.T) {
	fields := connect.Fields{
		URL:        "endpoint.url",
		Protocols:  "endpoint.protocols",
		Headers:    map[string]string{"X-Org": "org", "X-Other": "missing"},
		Credential: "auth.token",
	}
	const out = `{"endpoint":{"url":"wss://h/x?t=SECRET","protocols":["a","b"]},"org":"o1","auth":{"token":"SECRET"}}`
	var gotArgv, gotEnv []string
	run := func(_ context.Context, argv, env []string) ([]byte, error) {
		gotArgv, gotEnv = argv, env
		return []byte(out), nil
	}
	ep, err := connect.Resolve(context.Background(), run, []string{"tool", "connect"}, []string{"K=V"}, fields,
		[]string{"X-Org", "X-Other"})
	require.NoError(t, err)
	require.Equal(t, "wss://h/x?t=SECRET", ep.URL)
	require.Equal(t, []string{"a", "b"}, ep.Protocols)
	require.Equal(t, "o1", ep.Header.Get("X-Org"))
	require.Empty(t, ep.Header.Values("X-Other"))
	require.Equal(t, "Bearer SECRET", ep.Header.Get("Authorization"))
	require.Equal(t, []string{"tool", "connect"}, gotArgv)
	require.Equal(t, []string{"K=V"}, gotEnv)
}

func TestResolveRejectsHeaderOutsideAllowlist(t *testing.T) {
	fields := connect.Fields{URL: "url", Headers: map[string]string{"Cookie": "c"}}
	_, err := connect.Resolve(context.Background(), execReturning(`{"url":"wss://h","c":"v"}`, nil),
		[]string{"tool"}, nil, fields, []string{"X-Org"})
	require.ErrorContains(t, err, "Cookie")
}

func TestResolveErrorsNameCommandAndFieldNeverValues(t *testing.T) {
	fields := connect.Fields{URL: "url", Credential: "tok"}
	cases := map[string]struct {
		out, want string
		execErr   error
	}{
		"missing url":    {out: `{"tok":"SECRET"}`, want: "url"},
		"non-string url": {out: `{"url":["SECRET"]}`, want: "url"},
		"bad credential": {out: `{"url":"wss://h","tok":{"x":"SECRET"}}`, want: "tok"},
		"malformed":      {out: `{"url":"SECRET"`, want: "decode"},
		"two objects":    {out: `{"url":"wss://h"}{"url":"SECRET"}`, want: "one"},
		"not an object":  {out: `"SECRET"`, want: "decode"},
		"exec failure":   {execErr: errors.New("exit 1"), want: "exit 1"},
		"non-ws url":     {out: `{"url":"http://SECRET"}`, want: "url"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := connect.Resolve(context.Background(), execReturning(tc.out, tc.execErr),
				[]string{"mytool", "connect"}, nil, fields, nil)
			require.Error(t, err)
			require.Contains(t, err.Error(), "mytool")
			require.Contains(t, err.Error(), tc.want)
			require.False(t, strings.Contains(err.Error(), "SECRET"), err.Error())
		})
	}
}
