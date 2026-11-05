package domain

// ReviewSide is which side of a diff a review thread is anchored to.
type ReviewSide string

const (
	ReviewSideLeft  ReviewSide = "left"
	ReviewSideRight ReviewSide = "right"
)
