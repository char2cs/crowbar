package domain

// ReviewThreadStatus is the open↔resolved lifecycle.
type ReviewThreadStatus string

const (
	ReviewThreadStatusOpen     ReviewThreadStatus = "open"
	ReviewThreadStatusResolved ReviewThreadStatus = "resolved"
)
