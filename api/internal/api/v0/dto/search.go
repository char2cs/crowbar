package dto

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
