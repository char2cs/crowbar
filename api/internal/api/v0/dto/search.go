package dto

type SearchRequest struct {
	Query         string   `json:"query" binding:"required"`
	CaseSensitive bool     `json:"caseSensitive"`
	WholeWord     bool     `json:"wholeWord"`
	Regex         bool     `json:"regex"`
	Include       []string `json:"include"`
	Exclude       []string `json:"exclude"`
}

type ReplaceRequest struct {
	Query         string `json:"query" binding:"required"`
	Replacement   string `json:"replacement"`
	Scope         string `json:"scope"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWord     bool   `json:"wholeWord"`
	Regex         bool   `json:"regex"`
}
