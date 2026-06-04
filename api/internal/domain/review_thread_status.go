package domain

// ReviewThreadStatus is the open↔resolved lifecycle (00 §6.3).
type ReviewThreadStatus string

const (
	ReviewThreadStatusOpen     ReviewThreadStatus = "open"
	ReviewThreadStatusResolved ReviewThreadStatus = "resolved"
)
