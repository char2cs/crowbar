// Package exposure opens and closes, at runtime, the one TLS listener that lets
// other devices reach this daemon. The owner's choice is saved beside the home's
// state so it survives a restart; auth stays the bearer token, as for any
// network-exposed bind.
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

// DefaultAddress is the bind offered until the owner picks another.
const DefaultAddress = "0.0.0.0:3737"

// ErrPortInUse is what the owner is told when another program holds the port.
var ErrPortInUse = errors.New("That port is already in use. Pick another.") //nolint:staticcheck // ST1005: shown to the owner verbatim as a sentence

// Status is what the owner sees and what a pairing code carries.
type Status struct {
	// Enabled is whether the listener is open now.
	Enabled bool
	// Address is the configured bind, kept while the listener is closed.
	Address string
	// Bound is the address actually listening; empty while closed.
	Bound string
	// Reachable lists host:port pairs of this machine's own addresses, for a
	// wildcard bind; the bind itself otherwise.
	Reachable []string
	// Fingerprint is the TLS identity a device confirms; empty if it cannot be loaded.
	Fingerprint string
	// Error says why the last attempt to open failed.
	Error string
}

// Config is what New needs; every field is required.
type Config struct {
	// Root is the Crowbar home: the choice lives in network.json and the identity in tls/.
	Root string
	// NewServer builds the server for one opening, so closing it ends its connections.
	NewServer func() *http.Server
	// Listen binds addr (a tcp:// host) serving TLS with id.
	Listen func(addr string, id *tlsid.Identity) (net.Listener, error)
	// Interfaces lists this machine's own addresses.
	Interfaces func() ([]net.Addr, error)
}

// Exposure owns the exposed listener.
type Exposure struct {
	cfg Config

	mu       sync.Mutex
	identity *tlsid.Identity
	saved    choice
	srv      *http.Server
	streams  *hijacked
	ln       net.Listener
	// openAddr is what ln was asked to bind, which a serve flag may set apart from saved.
	openAddr string
	lastErr  string
	serving  sync.WaitGroup
}

// New builds a closed Exposure; Start applies the saved choice.
func New(cfg Config) *Exposure {
	return &Exposure{cfg: cfg}
}

// NetListen is the production Config.Listen: TLS on a non-loopback tcp:// bind.
func NetListen(addr string, id *tlsid.Identity) (net.Listener, error) {
	ln, err := transports.NewTCP(addr, id)
	if err != nil {
		return nil, fmt.Errorf("exposure: listen: %w", err)
	}
	return ln, nil
}

// Start opens the saved choice if it was on; a non-empty bootAddress (the serve
// flag) opens that instead and fails Start if it cannot. A bad or unopenable
// saved choice stays closed and shows in Status, so the local socket still serves.
func (e *Exposure) Start(bootAddress string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	saved, err := load(e.cfg.Root)
	if err == nil && saved.Enabled {
		// A hand-edited loopback bind would serve plaintext past the bearer guard.
		err = validate(saved.Address)
	}
	if err != nil {
		// An unusable choice leaves the listener closed, the safe side; Status says why.
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

// Enable closes any open listener, opens address and saves the choice.
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

// restoreLocked undoes a failed switch: the previous address reopens, or, if it
// cannot, the saved choice becomes off so the file never says what is not true.
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

// Disable closes the listener and its connections and saves the choice. Paired
// devices stay paired: this only stops listening.
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

// Status reports the listener and the identity.
func (e *Exposure) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.statusLocked()
}

// Close stops listening without changing the saved choice.
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
