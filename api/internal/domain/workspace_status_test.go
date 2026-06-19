package domain

import "testing"

func TestWorkspaceStatus_WireStrings(t *testing.T) {
	cases := []struct {
		status WorkspaceStatus
		wire   string
	}{
		{WorkspaceStatusNew, "new"},
		{WorkspaceStatusLocked, "locked"},
		{WorkspaceStatusPRConflicts, "pr-conflicts"},
		{WorkspaceStatusDeleted, "deleted"},
		{WorkspaceStatusPROpen, "pr-open"},
		{WorkspaceStatusPRMerged, "pr-merged"},
		{WorkspaceStatusPRClosed, "pr-closed"},
	}

	for _, tc := range cases {
		if string(tc.status) != tc.wire {
			t.Errorf("status %q != wire %q", string(tc.status), tc.wire)
		}
	}
}
