// Package avatar generates deterministic avatar labels and colors for repos from
// their names (00 §5.7). Pure functions — no state.
package avatar

import (
	"hash/fnv"
	"unicode"
)

// paletteSize is the number of entries in palette(); must equal len(palette()).
const paletteSize = 8

func palette() []string {
	return []string{
		"avatar-rose",
		"avatar-amber",
		"avatar-emerald",
		"avatar-cyan",
		"avatar-indigo",
		"avatar-violet",
		"avatar-slate",
		"avatar-pink",
	}
}

// Palette returns the avatar color token set.
func Palette() []string {
	return palette()
}

// Label returns the single-char avatar badge: first alphanumeric char of name,
// uppercased; "?" when none.
func Label(
	name string,
) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	return "?"
}

// Color returns a stable palette token hashed from name.
func Color(
	name string,
) string {
	p := palette()
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return p[h.Sum32()%paletteSize]
}
