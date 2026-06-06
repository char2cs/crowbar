package avatar

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPaletteSizeMatchesConst(t *testing.T) {
	assert.Len(t, Palette(), paletteSize)
}

func TestLabel_FirstAlnumUppercased(t *testing.T) {
	assert.Equal(t, "C", Label("crowbar"))
	assert.Equal(t, "9", Label("9front"))
	assert.Equal(t, "A", Label("  api"))
	assert.Equal(t, "?", Label(""))
	assert.Equal(t, "?", Label("---"))
}

func TestColor_StableForSameName(t *testing.T) {
	a := Color("crowbar")
	b := Color("crowbar")
	assert.Equal(t, a, b)
	assert.Contains(t, Palette(), a)
}

func TestColor_DistributesAcrossPalette(t *testing.T) {
	seen := map[string]bool{}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		seen[Color(n)] = true
	}
	assert.GreaterOrEqual(t, len(seen), 2)
}
