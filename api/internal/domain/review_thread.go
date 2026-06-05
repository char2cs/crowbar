package domain

import "time"

// ReviewThread is the branch-review comment-thread aggregate (00 §6.3).
type ReviewThread struct {
	ID        string             `json:"id"`
	WsID      string             `json:"wsId"`
	Status    ReviewThreadStatus `json:"status"`
	CreatedAt time.Time          `json:"createdAt"`
}
