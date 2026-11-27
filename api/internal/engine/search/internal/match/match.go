package match

import (
	"fmt"
	"regexp"
	"unicode/utf16"
)

type MatchSpan struct {
	Start int
	End   int
}

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

func byteToUTF16Col(
	runes []rune,
	byteOffset int,
) int {
	col := 0
	consumed := 0
	for _, r := range runes {
		if consumed >= byteOffset {
			break
		}
		runeLen := len(string(r))
		consumed += runeLen
		if r >= 0x10000 {
			encoded := utf16.Encode([]rune{r})
			col += len(encoded)
		} else {
			col++
		}
	}
	return col
}
