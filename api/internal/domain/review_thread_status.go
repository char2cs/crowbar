package domain

type ReviewThreadStatus string

const (
	ReviewThreadStatusOpen     ReviewThreadStatus = "open"
	ReviewThreadStatusResolved ReviewThreadStatus = "resolved"
)
