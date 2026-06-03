package domain

import "time"

type ReviewThreadStatus string
type ReviewPhase string

const (
	ReviewThreadStatusOpen          ReviewThreadStatus = "open"
	ReviewThreadStatusAgreed        ReviewThreadStatus = "agreed"
	ReviewThreadStatusForceApproved ReviewThreadStatus = "force_approved"

	ReviewPhaseAIReview    ReviewPhase = "ai_review"
	ReviewPhaseHumanReview ReviewPhase = "human_review"
)

type ReviewMessage struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"thread_id"`
	Role      string    `json:"role"`    // "reviewer" | "implementer" | "human"
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type ReviewThread struct {
	ID         string             `json:"id"`
	TaskID     string             `json:"task_id"`
	AgentRunID *string            `json:"agent_run_id,omitempty"`
	File       string             `json:"file"`
	Line       int                `json:"line"`
	Phase      ReviewPhase        `json:"phase"`
	OpenedBy   string             `json:"opened_by"` // "human" | "reviewer"
	Status     ReviewThreadStatus `json:"status"`
	Emoji      string             `json:"emoji,omitempty"`
	Messages   []ReviewMessage    `json:"messages"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}
