package repositories

import (
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/conversation"
	"github.com/char2cs/crowbar/api/internal/app/repositories/kanbanitem"
	"github.com/char2cs/crowbar/api/internal/app/repositories/project"
	"github.com/char2cs/crowbar/api/internal/app/repositories/repository"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/task"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ErrNotFound is returned by repository methods when a record does not exist.
var ErrNotFound = domain.ErrNotFound

// ErrBadRequest is returned when the caller supplies invalid input.
var ErrBadRequest = domain.ErrBadRequest

// Project is the repository interface for the Project aggregate.
type Project = project.Project

// Repository is the repository interface for the Repository aggregate.
type Repository = repository.Repository

// Conversation is the repository interface for ConversationMessage records.
type Conversation = conversation.Conversation

// Task is the repository interface for the Task aggregate.
type Task = task.Task

// AgentRun is the repository interface for the AgentRun aggregate.
type AgentRun = agentrun.AgentRun

// KanbanItem is the repository interface for the KanbanItem aggregate.
type KanbanItem = kanbanitem.KanbanItem

// ReviewThread is the repository interface for the ReviewThread aggregate.
type ReviewThread = reviewthread.ReviewThread
