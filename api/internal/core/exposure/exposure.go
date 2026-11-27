package exposure

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

const DefaultAddress = "0.0.0.0:3737"

var ErrPortInUse = errors.New("That port is already in use. Pick another.") //nolint:staticcheck // ST1005: shown to the owner verbatim as a sentence

type Status struct {
	Enabled     bool
	Address     string
	Bound       string
	Reachable   []string
	Fingerprint string
	Error       string
}

type Config struct {
	Root       string
	NewServer  func() *http.Server
	Listen     func(addr string, id *tlsid.Identity) (net.Listener, error)
	Interfaces func() ([]net.Addr, error)
}

type Exposure struct {
	cfg Config

	mu       sync.Mutex
	identity *tlsid.Identity
	saved    choice
	srv      *http.Server
	streams  *hijacked
	ln       net.Listener
	openAddr string
	lastErr  string
	serving  sync.WaitGroup
}

func New(cfg Config) *Exposure {
	return &Exposure{cfg: cfg}
}

func NetListen(addr string, id *tlsid.Identity) (net.Listener, error) {
	ln, err := transports.NewTCP(addr, id)
	if err != nil {
		return nil, fmt.Errorf("exposure: listen: %w", err)
	}
	return ln, nil
}

func (e *Exposure) Start(bootAddress string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	saved, err := load(e.cfg.Root)
	if err == nil && saved.Enabled {
		err = validate(saved.Address)
	}
	if err != nil {
		e.lastErr = err.Error()
		slog.Warn("exposure: saved choice unusable", "component", "exposure", "err", err)
		saved = choice{Address: DefaultAddress}
	}
	e.saved = saved
	address, open := saved.Address, saved.Enabled
	if bootAddress != "" {
		address, open = strings.TrimPrefix(bootAddress, "tcp://"), true
	}
	if !open {
		return nil
	}
	if err := e.open(address); err != nil {
		e.lastErr = err.Error()
		if bootAddress != "" {
			return err
		}
		slog.Warn("exposure: could not open the device listener", "component", "exposure", "address", address, "err", err)
	}
	return nil
}

func (e *Exposure) Enable(address string) (Status, error) {
	if err := validate(address); err != nil {
		return e.Status(), err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	previous, wasOpen := e.openAddr, e.ln != nil
	e.closeLocked()
	if err := e.open(address); err != nil {
		return e.restoreLocked(previous, wasOpen, err)
	}
	next := choice{Enabled: true, Address: address}
	if err := save(e.cfg.Root, next); err != nil {
		return e.restoreLocked(previous, wasOpen, err)
	}
	e.saved = next
	return e.statusLocked(), nil
}

func (e *Exposure) restoreLocked(previous string, wasOpen bool, cause error) (Status, error) {
	e.closeLocked()
	if wasOpen && e.open(previous) != nil {
		e.saved.Enabled = false
		if err := save(e.cfg.Root, e.saved); err != nil {
			slog.Warn("exposure: save after failed switch", "component", "exposure", "err", err)
		}
	}
	e.lastErr = cause.Error()
	return e.statusLocked(), cause
}

func (e *Exposure) Disable() (Status, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeLocked()
	e.saved.Enabled = false
	if e.saved.Address == "" {
		e.saved.Address = DefaultAddress
	}
	if err := save(e.cfg.Root, e.saved); err != nil {
		return e.statusLocked(), err
	}
	return e.statusLocked(), nil
}

func (e *Exposure) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.statusLocked()
}

func (e *Exposure) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeLocked()
}

func (e *Exposure) statusLocked() Status {
	st := Status{Address: e.saved.Address, Error: e.lastErr}
	if st.Address == "" {
		st.Address = DefaultAddress
	}
	if id, err := e.identityLocked(); err != nil {
		st.Error = err.Error()
	} else {
		st.Fingerprint = id.Fingerprint
	}
	if e.ln != nil {
		st.Enabled = true
		st.Address = e.openAddr
		st.Bound = e.ln.Addr().String()
		st.Reachable = reachable(st.Bound, e.cfg.Interfaces)
	}
	return st
}

func (e *Exposure) identityLocked() (*tlsid.Identity, error) {
	if e.identity != nil {
		return e.identity, nil
	}
	id, err := tlsid.Load(filepath.Join(e.cfg.Root, "tls"))
	if err != nil {
		return nil, fmt.Errorf("exposure: identity: %w", err)
	}
	e.identity = id
	return id, nil
}

func (e *Exposure) open(address string) error {
	id, err := e.identityLocked()
	if err != nil {
		return err
	}
	ln, err := e.cfg.Listen("tcp://"+address, id)
	if errors.Is(err, syscall.EADDRINUSE) {
		slog.Warn("exposure: port in use", "component", "exposure", "address", address, "err", err)
		return ErrPortInUse
	}
	if err != nil {
		return err
	}
	srv := e.cfg.NewServer()
	streams := &hijacked{}
	streams.watch(srv)
	e.srv, e.streams, e.ln, e.openAddr, e.lastErr = srv, streams, ln, address, ""
	e.serving.Go(func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("exposure: serve ended", "component", "exposure", "err", err)
		}
	})
	return nil
}

func (e *Exposure) closeLocked() {
	if e.srv == nil {
		return
	}
	if err := e.srv.Close(); err != nil {
		slog.Warn("exposure: close", "component", "exposure", "err", err)
	}
	e.streams.closeAll()
	e.serving.Wait()
	e.srv, e.streams, e.ln = nil, nil, nil
}
