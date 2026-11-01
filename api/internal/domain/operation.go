package domain

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// OperationKind names the work an operation runs; the set is closed.
type OperationKind string

// The operation kinds.
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
	// OpEntitySave is the daemon giving up on saving an entity's changes.
	OpEntitySave OperationKind = "entity.save"
)

// OperationState is where an operation is in accepted → running → succeeded | failed.
type OperationState string

// The operation states.
const (
	OpAccepted  OperationState = "accepted"
	OpRunning   OperationState = "running"
	OpSucceeded OperationState = "succeeded"
	OpFailed    OperationState = "failed"
)

// EntityKind is the kind of entity an operation names.
type EntityKind string

// The entity kinds.
const (
	EntityProject   EntityKind = "project"
	EntityRepo      EntityKind = "repo"
	EntityWorkspace EntityKind = "workspace"
	EntityChat      EntityKind = "chat"
)

// EntityRef names one entity.
type EntityRef struct {
	Kind EntityKind `json:"kind"`
	ID   string     `json:"id"`
}

// OperationItem is one entity an operation creates or changes, with its own outcome.
type OperationItem struct {
	Ref   EntityRef        `json:"ref"`
	Label string           `json:"label"`
	State OperationState   `json:"state"`
	Error *problem.Problem `json:"error,omitempty"`
}

// Operation is work that outlives its request, and its outcome.
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
	// The service's clock owns both times; gorm must not stamp its own.
	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime:false"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"index;autoUpdateTime:false"`
}
