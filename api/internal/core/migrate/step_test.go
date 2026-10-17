package migrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhase_String(t *testing.T) {
	assert.Equal(t, "pre-open", PreOpen.String())
	assert.Equal(t, "post-open", PostOpen.String())
	assert.Equal(t, "phase(0)", Phase(0).String())
}

func TestCounts_AddSums(t *testing.T) {
	c := Counts{"moved": 2, "kept": 1}

	c.Add(Counts{"moved": 3, "deleted": 4})

	assert.Equal(t, Counts{"moved": 5, "kept": 1, "deleted": 4}, c)
}

func TestStep_IDCarriesNumberAndName(t *testing.T) {
	s := Step{Number: 7, Name: "chat-types"}

	assert.Equal(t, ID{Number: 7, Name: "chat-types"}, s.ID())
}
