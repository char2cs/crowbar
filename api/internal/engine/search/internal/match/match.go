// Package match provides pattern compilation and line-level search primitives.
package match

import (
	"fmt"
	"regexp"
	"unicode/utf16"
)

// MatchSpan holds UTF-16 column offsets for a single match within a line.
type MatchSpan struct {
	Start int
	End   int
}

// BuildPattern compiles a *regexp.Regexp from the search parameters.
//
// Fixed-string mode (Regex=false) escapes the query with regexp.QuoteMeta.
// WholeWord wraps the pattern with \b word boundaries.
// CaseSensitive=false prepends (?i).
func BuildPattern(
	query string,
	caseSensitive bool,
	wholeWord bool,
	regex bool,
) (*regexp.Regexp, error) {
	if query == "" {
		return nil, fmt.Errorf("match: build pattern: query must not be empty")
	}

	pat := query
	if !regex {
		pat = regexp.QuoteMeta(query)
	}

	if wholeWord {
		pat = `\b` + pat + `\b`
	}

	if !caseSensitive {
		pat = `(?i)` + pat
	}

	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("match: build pattern: %w", err)
	}

	return re, nil
}

// FindMatches returns all match spans within lineText as UTF-16 column offsets.
func FindMatches(
	re *regexp.Regexp,
	lineText string,
) []MatchSpan {
	pairs := re.FindAllStringIndex(lineText, -1)
	if len(pairs) == 0 {
		return nil
	}

	runes := []rune(lineText)
	spans := make([]MatchSpan, 0, len(pairs))

	for _, pair := range pairs {
		start := byteToUTF16Col(runes, pair[0])
		end := byteToUTF16Col(runes, pair[1])
		spans = append(spans, MatchSpan{Start: start, End: end})
	}

	return spans
}

// IsBinary reports whether data looks like binary content.
// A null byte anywhere in the sample is sufficient evidence.
func IsBinary(
	sample []byte,
) bool {
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	return false
}

// byteToUTF16Col converts a byte offset in a UTF-8 string to a UTF-16 code unit offset.
func byteToUTF16Col(
	runes []rune,
	byteOffset int,
) int {
	// Count bytes consumed by each rune until we reach byteOffset.
	col := 0
	consumed := 0
	for _, r := range runes {
		if consumed >= byteOffset {
			break
		}
		runeLen := len(string(r))
		consumed += runeLen
		// A surrogate pair in UTF-16 counts as 2 code units.
		if r >= 0x10000 {
			encoded := utf16.Encode([]rune{r})
			col += len(encoded)
		} else {
			col++
		}
	}
	return col
}
