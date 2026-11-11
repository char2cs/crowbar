package watch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBurst_MergesOpsPerPathInFirstSeenOrder(t *testing.T) {
	b := newBurst()
	t.Cleanup(b.stop)

	b.add("a", opCreate)
	b.add("b", opWrite)
	b.add("a", opWrite)

	assert.Equal(t, []event{{Name: "a", Op: opCreate | opWrite}, {Name: "b", Op: opWrite}}, b.take())
	assert.Empty(t, b.take(), "take empties the burst")
}
