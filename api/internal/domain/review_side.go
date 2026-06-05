package domain

// ReviewSide is which side of a diff a review thread is anchored to (09 §3).
type ReviewSide string

const (
	ReviewSideLeft  ReviewSide = "left"
	ReviewSideRight ReviewSide = "right"
)
