package accessfake

import "github.com/char2cs/crowbar/api/internal/core/exposure"

type FakeAccess struct {
	St exposure.Status
}

func Closed() *FakeAccess {
	return &FakeAccess{St: exposure.Status{Address: exposure.DefaultAddress}}
}

func Listening(fingerprint string) *FakeAccess {
	return &FakeAccess{St: exposure.Status{Enabled: true, Address: exposure.DefaultAddress, Fingerprint: fingerprint}}
}

func (f *FakeAccess) Status() exposure.Status { return f.St }

func (f *FakeAccess) Enable(address string) (exposure.Status, error) {
	f.St.Enabled, f.St.Address = true, address
	return f.St, nil
}

func (f *FakeAccess) Disable() (exposure.Status, error) {
	f.St.Enabled = false
	return f.St, nil
}
