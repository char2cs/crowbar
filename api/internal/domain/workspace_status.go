package domain

// WorkspaceStatus is the base lifecycle badge; "" means "has commits, no PR" (00 §6.1).
type WorkspaceStatus string

const (
	WorkspaceStatusNew         WorkspaceStatus = "new"
	WorkspaceStatusLocked      WorkspaceStatus = "locked"
	WorkspaceStatusPRConflicts WorkspaceStatus = "pr-conflicts"
	WorkspaceStatusDeleted     WorkspaceStatus = "deleted"
	WorkspaceStatusPROpen      WorkspaceStatus = "pr-open"
	WorkspaceStatusPRMerged    WorkspaceStatus = "pr-merged"
	WorkspaceStatusPRClosed    WorkspaceStatus = "pr-closed"
)
