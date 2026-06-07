package dto

import (
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

// SearchRequest is the body for the global content search route: the query and
// its modifiers plus the optional include/exclude doublestar globs.
type SearchRequest struct {
	Query         string   `json:"query" binding:"required"`
	CaseSensitive bool     `json:"caseSensitive"`
	WholeWord     bool     `json:"wholeWord"`
	Regex         bool     `json:"regex"`
	Include       []string `json:"include"`
	Exclude       []string `json:"exclude"`
}

// ReplaceRequest is the body for the search-and-replace route: the query, its
// replacement text, the affected scope ("all" or "file:<path>"), and the match
// modifiers.
type ReplaceRequest struct {
	Query         string `json:"query" binding:"required"`
	Replacement   string `json:"replacement"`
	Scope         string `json:"scope"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWord     bool   `json:"wholeWord"`
	Regex         bool   `json:"regex"`
}

// SearchResultDTO is one match within a file: the workspace-relative path, the
// one-based line number, the full line text, and the UTF-16 column span of the
// match suitable for Monaco/LSP clients.
type SearchResultDTO struct {
	FilePath   string `json:"filePath"`
	LineNumber int    `json:"lineNumber"`
	LineText   string `json:"lineText"`
	MatchStart int    `json:"matchStart"`
	MatchEnd   int    `json:"matchEnd"`
}

// SearchResponseDTO is the search payload carried in the response envelope's
// data field: the match list (never null, empty on no hits) and the truncation
// flag set when the 1000-result cap is reached.
type SearchResponseDTO struct {
	Results   []SearchResultDTO `json:"results"`
	Truncated bool              `json:"truncated"`
}

// SearchResponseToDTO converts an engine SearchResponse into its wire shape,
// guaranteeing a non-null Results slice.
func SearchResponseToDTO(
	resp enginesearch.SearchResponse,
) SearchResponseDTO {
	results := make([]SearchResultDTO, 0, len(resp.Results))
	for _, r := range resp.Results {
		results = append(results, SearchResultDTO{
			FilePath:   r.FilePath,
			LineNumber: r.LineNumber,
			LineText:   r.LineText,
			MatchStart: r.MatchStart,
			MatchEnd:   r.MatchEnd,
		})
	}
	return SearchResponseDTO{
		Results:   results,
		Truncated: resp.Truncated,
	}
}
