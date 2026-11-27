package domain

type WorkspaceProvisioning string

const (
	WorkspaceProvisioned WorkspaceProvisioning = "provisioned"
	WorkspacePlaceholder WorkspaceProvisioning = "placeholder"
	WorkspaceShared      WorkspaceProvisioning = "shared"
)

func (p WorkspaceProvisioning) HasWorktree() bool {
	return p == WorkspaceProvisioned || p == WorkspaceShared
}
