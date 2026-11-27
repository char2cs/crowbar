package exposure_test

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/exposure"
	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

func loopbackTLS(_ string, id *tlsid.Identity) (net.Listener, error) {
	raw, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	secured := tls.NewListener(raw, &tls.Config{Certificates: []tls.Certificate{id.Certificate}, MinVersion: tls.VersionTLS13})
	tcp, ok := raw.Addr().(*net.TCPAddr)
	if !ok {
		return nil, errors.New("not a TCP listener")
	}
	return wildcard{Listener: secured, port: tcp.Port}, nil
}

type wildcard struct {
	net.Listener
	port int
}

func (w wildcard) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4zero, Port: w.port} }

func newExposure(t *testing.T, root string) *exposure.Exposure {
	t.Helper()
	e := exposure.New(exposure.Config{
		Root: root,
		NewServer: func() *http.Server {
			return &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("gate"))
			})}
		},
		Listen: loopbackTLS,
		Interfaces: func() ([]net.Addr, error) {
			return []net.Addr{mustCIDR(t, "192.168.1.20/24"), mustCIDR(t, "127.0.0.1/8")}, nil
		},
	})
	t.Cleanup(e.Close)
	return e
}

func mustCIDR(t *testing.T, s string) net.Addr {
	t.Helper()
	ip, n, err := net.ParseCIDR(s)
	require.NoError(t, err)
	n.IP = ip
	return n
}

func get(t *testing.T, addr string) (string, string, error) {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+addr, http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	return string(body), tlsid.FingerprintOf(resp.TLS.PeerCertificates[0].Raw), nil
}

func TestRegression_Exposure_OffByDefaultWithFingerprint(t *testing.T) {
	e := newExposure(t, t.TempDir())
	require.NoError(t, e.Start(""))

	st := e.Status()
	assert.False(t, st.Enabled)
	assert.Equal(t, exposure.DefaultAddress, st.Address)
	assert.Empty(t, st.Reachable)
	assert.NotEmpty(t, st.Fingerprint)
}

func TestRegression_Exposure_EnableServesTLSWithTheIdentity(t *testing.T) {
	e := newExposure(t, t.TempDir())
	require.NoError(t, e.Start(""))

	st, err := e.Enable("0.0.0.0:3737")
	require.NoError(t, err)

	require.True(t, st.Enabled)
	require.Len(t, st.Reachable, 1)
	assert.Contains(t, st.Reachable[0], "192.168.1.20:")
	body, fp, err := get(t, loopback(e.Status().Bound))
	require.NoError(t, err)
	assert.Equal(t, "gate", body)
	assert.Equal(t, st.Fingerprint, fp)
}

func TestRegression_Exposure_DisableClosesThePortAndLiveConnections(t *testing.T) {
	e := newExposure(t, t.TempDir())
	require.NoError(t, e.Start(""))
	_, err := e.Enable("0.0.0.0:3737")
	require.NoError(t, err)
	bound := e.Status().Bound
	conn, err := (&tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}}).DialContext(t.Context(), "tcp", loopback(bound))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	st, err := e.Disable()
	require.NoError(t, err)

	assert.False(t, st.Enabled)
	_, err = conn.Read(make([]byte, 1))
	require.Error(t, err, "an open connection must end")
	_, _, err = get(t, loopback(bound))
	assert.Error(t, err, "the port must be closed")
}

func TestRegression_Exposure_ChoiceSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	first := newExposure(t, root)
	require.NoError(t, first.Start(""))
	_, err := first.Enable("0.0.0.0:3737")
	require.NoError(t, err)
	first.Close()

	second := newExposure(t, root)
	require.NoError(t, second.Start(""))
	assert.True(t, second.Status().Enabled, "on stays on")
	_, err = second.Disable()
	require.NoError(t, err)
	second.Close()

	third := newExposure(t, root)
	require.NoError(t, third.Start(""))
	assert.False(t, third.Status().Enabled, "off stays off")
	assert.Equal(t, "0.0.0.0:3737", third.Status().Address, "the chosen address is kept while off")
}

func TestRegression_Exposure_RefusesALoopbackBind(t *testing.T) {
	e := newExposure(t, t.TempDir())
	require.NoError(t, e.Start(""))

	for _, addr := range []string{"127.0.0.1:3737", "localhost:3737", "[::1]:3737", "nonsense", "0.0.0.0:0", "0.0.0.0:99999"} {
		_, err := e.Enable(addr)
		require.Errorf(t, err, "%s", addr)
	}
	assert.False(t, e.Status().Enabled)
}

func TestRegression_Exposure_StartAddressOpensWithoutPersisting(t *testing.T) {
	root := t.TempDir()
	e := newExposure(t, root)
	require.NoError(t, e.Start("tcp://0.0.0.0:4000"))
	assert.True(t, e.Status().Enabled)
	assert.Equal(t, "0.0.0.0:4000", e.Status().Address)
	e.Close()

	again := newExposure(t, root)
	require.NoError(t, again.Start(""))
	assert.False(t, again.Status().Enabled)
}

func TestRegression_Exposure_FailedOpenIsReportedAndRetainsTheChoice(t *testing.T) {
	root := t.TempDir()
	first := newExposure(t, root)
	require.NoError(t, first.Start(""))
	_, err := first.Enable("0.0.0.0:3737")
	require.NoError(t, err)
	first.Close()

	failing := exposure.New(exposure.Config{
		Root:       root,
		NewServer:  func() *http.Server { return &http.Server{} },
		Listen:     func(string, *tlsid.Identity) (net.Listener, error) { return nil, assert.AnError },
		Interfaces: func() ([]net.Addr, error) { return nil, nil },
	})
	t.Cleanup(failing.Close)
	require.NoError(t, failing.Start(""))

	st := failing.Status()
	assert.False(t, st.Enabled)
	assert.NotEmpty(t, st.Error)

	healthy := newExposure(t, root)
	require.NoError(t, healthy.Start(""))
	assert.True(t, healthy.Status().Enabled)
}

func loopback(bound string) string {
	_, port, _ := net.SplitHostPort(bound)
	return net.JoinHostPort("127.0.0.1", port)
}

func TestRegression_Exposure_UnreadableChoiceStaysClosed(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "network.json"), []byte("{nope"), 0o600))
	e := newExposure(t, root)

	require.NoError(t, e.Start(""))

	st := e.Status()
	assert.False(t, st.Enabled)
	assert.NotEmpty(t, st.Error)
}

func TestRegression_Exposure_DisableEndsHijackedStreams(t *testing.T) {
	streamEnded := make(chan struct{})
	e := exposure.New(exposure.Config{
		Root: t.TempDir(),
		NewServer: func() *http.Server {
			return &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					return
				}
				conn, rw, err := hijacker.Hijack()
				if err != nil {
					return
				}
				defer func() { _ = conn.Close() }()
				_, _ = rw.WriteString("HTTP/1.1 200 OK\r\n\r\nopen")
				_ = rw.Flush()
				_, _ = io.Copy(io.Discard, conn)
				close(streamEnded)
			})}
		},
		Listen:     loopbackTLS,
		Interfaces: func() ([]net.Addr, error) { return nil, nil },
	})
	t.Cleanup(e.Close)
	require.NoError(t, e.Start(""))
	_, err := e.Enable("0.0.0.0:3737")
	require.NoError(t, err)
	conn, err := (&tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}}).DialContext(t.Context(), "tcp", loopback(e.Status().Bound))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
	require.NoError(t, err)
	buf := make([]byte, 64)
	_, err = conn.Read(buf)
	require.NoError(t, err, "the stream is open")

	_, err = e.Disable()
	require.NoError(t, err)

	select {
	case <-streamEnded:
	case <-time.After(5 * time.Second):
		t.Fatal("a hijacked stream outlived Disable")
	}
}

func TestRegression_Exposure_FailedEnableKeepsThePreviousAddress(t *testing.T) {
	root := t.TempDir()
	e := exposure.New(exposure.Config{
		Root:      root,
		NewServer: func() *http.Server { return &http.Server{} },
		Listen: func(addr string, id *tlsid.Identity) (net.Listener, error) {
			if addr == "tcp://0.0.0.0:4000" {
				return nil, assert.AnError
			}
			return loopbackTLS(addr, id)
		},
		Interfaces: func() ([]net.Addr, error) { return nil, nil },
	})
	t.Cleanup(e.Close)
	require.NoError(t, e.Start(""))
	_, err := e.Enable("0.0.0.0:3737")
	require.NoError(t, err)

	st, err := e.Enable("0.0.0.0:4000")

	require.Error(t, err)
	assert.True(t, st.Enabled, "the previous listener is back")
	assert.Equal(t, "0.0.0.0:3737", st.Address)
	assert.NotEmpty(t, st.Error)
	again := newExposure(t, root)
	require.NoError(t, again.Start(""))
	assert.True(t, again.Status().Enabled)
	assert.Equal(t, "0.0.0.0:3737", again.Status().Address)
}

func TestRegression_Exposure_FailedEnableThatCannotRestoreSavesOff(t *testing.T) {
	root := t.TempDir()
	opens := 0
	e := exposure.New(exposure.Config{
		Root:      root,
		NewServer: func() *http.Server { return &http.Server{} },
		Listen: func(addr string, id *tlsid.Identity) (net.Listener, error) {
			opens++
			if opens > 1 {
				return nil, assert.AnError
			}
			return loopbackTLS(addr, id)
		},
		Interfaces: func() ([]net.Addr, error) { return nil, nil },
	})
	t.Cleanup(e.Close)
	require.NoError(t, e.Start(""))
	_, err := e.Enable("0.0.0.0:3737")
	require.NoError(t, err)

	st, err := e.Enable("0.0.0.0:4000")

	require.Error(t, err)
	assert.False(t, st.Enabled)
	again := newExposure(t, root)
	require.NoError(t, again.Start(""))
	assert.False(t, again.Status().Enabled, "the file matches reality")
}

func TestRegression_Exposure_SavedLoopbackChoiceStaysClosed(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "network.json"), []byte(`{"enabled":true,"address":"127.0.0.1:3737"}`), 0o600))
	listened := false
	e := exposure.New(exposure.Config{
		Root:      root,
		NewServer: func() *http.Server { return &http.Server{} },
		Listen: func(string, *tlsid.Identity) (net.Listener, error) {
			listened = true
			return nil, assert.AnError
		},
		Interfaces: func() ([]net.Addr, error) { return nil, nil },
	})
	t.Cleanup(e.Close)

	require.NoError(t, e.Start(""))

	assert.False(t, listened)
	st := e.Status()
	assert.False(t, st.Enabled)
	assert.Contains(t, st.Error, "loopback")
}

func TestRegression_Exposure_PortInUseIsAPlainTypedError(t *testing.T) {
	taken, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = taken.Close() })
	e := exposure.New(exposure.Config{
		Root:      t.TempDir(),
		NewServer: func() *http.Server { return &http.Server{} },
		Listen: func(_ string, _ *tlsid.Identity) (net.Listener, error) {
			return (&net.ListenConfig{}).Listen(context.Background(), "tcp", taken.Addr().String())
		},
		Interfaces: func() ([]net.Addr, error) { return nil, nil },
	})
	t.Cleanup(e.Close)
	require.NoError(t, e.Start(""))

	st, err := e.Enable("0.0.0.0:3737")

	require.ErrorIs(t, err, exposure.ErrPortInUse)
	assert.Equal(t, "That port is already in use. Pick another.", err.Error())
	assert.Equal(t, err.Error(), st.Error)
	assert.False(t, st.Enabled)
}
