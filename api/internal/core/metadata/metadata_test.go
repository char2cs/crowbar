package metadata

import (
	"runtime/debug"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGet_LoadsEmbeddedMetadata(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	m := Get()
	assert.Equal(t, "Crowbar", m.Metadata.Name)
	assert.Equal(t, "0.1.0", m.Version.Number)
}

func TestGetVersion_NonEmpty(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	assert.NotEmpty(t, GetVersion())
}

func TestDefaultMetadata_Name(t *testing.T) {
	m := defaultMetadata()
	assert.Equal(t, "Crowbar", m.Metadata.Name)
	assert.Equal(t, "0.1.0", m.Version.Number)
}

func resetForTesting() {
	metadata = nil
	once = sync.Once{}
}

// A binary built in a checkout names the commit; the web and the desktop shell
// compare it with their own, so one built anywhere else must say so.
func TestBuildOf_NamesTheCommitOrDev(t *testing.T) {
	stamped := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs", Value: "git"}, {Key: "vcs.revision", Value: "abc123"}, {Key: "vcs.modified", Value: "true"},
	}}
	assert.Equal(t, "abc123", buildOf(stamped, true))
	assert.Equal(t, "dev", buildOf(&debug.BuildInfo{}, true))
	assert.Equal(t, "dev", buildOf(nil, false))
}
