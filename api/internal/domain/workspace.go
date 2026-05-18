package domain

import "time"

type Phase string

const (
	PhaseBrainstorm     Phase = "brainstorm"
	PhaseSpec           Phase = "spec"
	PhaseImplementation Phase = "implementation"
	PhaseAIReview       Phase = "ai_review"
	PhaseHumanReview    Phase = "human_review"
)

type Workspace struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Phase     Phase     `json:"phase"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
