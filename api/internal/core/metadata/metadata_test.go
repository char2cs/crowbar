package metadata

import (
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
