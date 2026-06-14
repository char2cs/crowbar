// Package avatar generates deterministic avatar labels and colors for repos from
// their names (00 §5.7). Pure functions — no state.
package avatar

import (
	"hash/fnv"
	"os"
	"path/filepath"
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

// iconCandidates is the ordered list of relative paths checked when scanning
// a repo root for an icon file. First match wins.
var iconCandidates = []string{
	"favicon.svg",
	"favicon.ico",
	"favicon.png",
	"logo.svg",
	"logo.png",
	"public/logo.svg",
	"public/logo.png",
	"public/favicon.svg",
	"public/favicon.ico",
	"public/favicon.png",
	"src/assets/logo.svg",
	"src/assets/logo.png",
}

// ScanRepoIcon walks iconCandidates relative to repoPath and returns the
// absolute path of the first file found, or "" when none match.
func ScanRepoIcon(repoPath string) string {
	for _, rel := range iconCandidates {
		abs := filepath.Join(repoPath, rel)
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return ""
}
