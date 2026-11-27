package domain

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type OperationKind string

const (
	OpWorkspaceSync             OperationKind = "workspace.sync"
	OpWorkspaceMergeIntoParent  OperationKind = "workspace.merge_into_parent"
	OpWorkspaceRebaseOntoParent OperationKind = "workspace.rebase_onto_parent"
	OpWorkspaceReparent         OperationKind = "workspace.reparent"
	OpWorkspaceRetryProvision   OperationKind = "workspace.retry_provision" //nolint:gosec // G101: an operation kind name, not a credential
	OpWorkspaceDetachHolder     OperationKind = "workspace.detach_holder"
	OpWorkspacePurge            OperationKind = "workspace.purge"
	OpGitPush                   OperationKind = "git.push"
	OpGitFetch                  OperationKind = "git.fetch"
	OpGitPull                   OperationKind = "git.pull"
	OpGitMerge                  OperationKind = "git.merge"
	OpGitRebase                 OperationKind = "git.rebase"
	OpBranchImport              OperationKind = "branch.import"
	OpRepoCreate                OperationKind = "repo.create"
	OpRepoDelete                OperationKind = "repo.delete"
	OpProjectCreate             OperationKind = "project.create"
	OpProjectDelete             OperationKind = "project.delete"
	OpEntitySave                OperationKind = "entity.save"
)

type OperationState string

const (
	OpAccepted  OperationState = "accepted"
	OpRunning   OperationState = "running"
	OpSucceeded OperationState = "succeeded"
	OpFailed    OperationState = "failed"
)

type EntityKind string

const (
	EntityProject   EntityKind = "project"
	EntityRepo      EntityKind = "repo"
	EntityWorkspace EntityKind = "workspace"
	EntityChat      EntityKind = "chat"
)

type EntityRef struct {
	Kind EntityKind `json:"kind"`
	ID   string     `json:"id"`
}

type OperationItem struct {
	Ref   EntityRef        `json:"ref"`
	Label string           `json:"label"`
	State OperationState   `json:"state"`
	Error *problem.Problem `json:"error,omitempty"`
}

type Operation struct {
	ID        string           `json:"id" gorm:"primaryKey"`
	Kind      OperationKind    `json:"kind"`
	Subject   EntityRef        `json:"subject" gorm:"embedded;embeddedPrefix:subject_"`
	ProjectID string           `json:"projectId" gorm:"index"`
	Label     string           `json:"label"`
	State     OperationState   `json:"state"`
	Items     []OperationItem  `json:"items" gorm:"serializer:json"`
	Error     *problem.Problem `json:"error,omitempty" gorm:"serializer:json"`
	Dismissed bool             `json:"dismissed"`
	Version   int              `json:"version"`
	CreatedAt time.Time        `json:"createdAt" gorm:"autoCreateTime:false"`
	UpdatedAt time.Time        `json:"updatedAt" gorm:"index;autoUpdateTime:false"`
}
