// Package accessfake is an in-memory device listener for tests that mount the API.
package accessfake

import "github.com/char2cs/crowbar/api/internal/core/exposure"

// FakeAccess is a device listener held in memory.
type FakeAccess struct {
	St exposure.Status
}

// Closed is a daemon with device connections off: it mints no pairing codes.
func Closed() *FakeAccess {
	return &FakeAccess{St: exposure.Status{Address: exposure.DefaultAddress}}
}

// Listening is a daemon with device connections on, carrying fingerprint.
func Listening(fingerprint string) *FakeAccess {
	return &FakeAccess{St: exposure.Status{Enabled: true, Address: exposure.DefaultAddress, Fingerprint: fingerprint}}
}

// Status reports the held state.
func (f *FakeAccess) Status() exposure.Status { return f.St }

// Enable records address as open.
func (f *FakeAccess) Enable(address string) (exposure.Status, error) {
	f.St.Enabled, f.St.Address = true, address
	return f.St, nil
}

// Disable records the listener as closed.
func (f *FakeAccess) Disable() (exposure.Status, error) {
	f.St.Enabled = false
	return f.St, nil
}
