package usecases

import (
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// Container holds all fully-constructed usecases.
type Container struct {
	Project      ProjectUsecase
	Repository   RepositoryUsecase
	Task         TaskUsecase
	AgentRun     AgentRunUsecase
	KanbanItem   KanbanItemUsecase
	ReviewThread ReviewThreadUsecase
	Conversation ConversationUsecase
	Git          GitUsecase
	Health       *HealthUsecase
}

// New constructs all usecases wired to the provided repositories and flow loader.
func New(
	repos *repositories.Container,
	flowLoader flow.Loader,
) *Container {
	return &Container{
		Project:      NewProject(repos.Project),
		Repository:   NewRepository(repos.Repository, repos.Project),
		Task:         NewTask(repos.Task, repos.Repository, flowLoader),
		AgentRun:     NewAgentRun(repos.AgentRun),
		KanbanItem:   NewKanbanItem(repos.KanbanItem),
		ReviewThread: NewReviewThread(repos.ReviewThread),
		Conversation: NewConversation(repos.Conversation),
		Git:          NewGit(repos.Task),
		Health:       NewHealth(),
	}
}
