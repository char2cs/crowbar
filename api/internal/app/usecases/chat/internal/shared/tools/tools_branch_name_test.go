package tools_test

import (
	"context"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeWorkspaceBranchRenamer struct {
	calledWithWsID   string
	calledWithBranch string
	err              error
}

func (f *fakeWorkspaceBranchRenamer) RenameBranch(
	ctx context.Context,
	wsID string,
	newBranch string,
) (domain.Workspace, error) {
	f.calledWithWsID = wsID
	f.calledWithBranch = newBranch
	return domain.Workspace{ID: wsID, Branch: newBranch}, f.err
}

func TestSetBranchName_RejectsEmptyName(t *testing.T) {
	res, minter := resolverOn(t, "ws-a")
	fakeRenamer := &fakeWorkspaceBranchRenamer{}
	ts := tools.NewToolSet(tools.Deps{
		Resolver:   res,
		Workspaces: fakeRenamer,
	}, "RUN", minter.Mint("RUN"))

	_, err := ts.Call(context.Background(), "set_branch_name", []byte(`{"name":""}`))
	if err == nil {
		t.Fatalf("expected error: agenttools: set_branch_name: name must not be empty")
	}
	if err.Error() != "agenttools: set_branch_name: name must not be empty" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetBranchName_CallsRenameBranch(t *testing.T) {
	res, minter := resolverOn(t, "ws-a")
	fakeRenamer := &fakeWorkspaceBranchRenamer{}
	ts := tools.NewToolSet(tools.Deps{
		Resolver:   res,
		Workspaces: fakeRenamer,
	}, "RUN", minter.Mint("RUN"))

	out, err := ts.Call(context.Background(), "set_branch_name", []byte(`{"name":"fix-the-thing"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if fakeRenamer.calledWithWsID != "ws-a" {
		t.Fatalf("renamer not called with correct workspace ID: got %q, want %q", fakeRenamer.calledWithWsID, "ws-a")
	}
	if fakeRenamer.calledWithBranch != "fix-the-thing" {
		t.Fatalf("renamer not called with correct branch name: got %q, want %q", fakeRenamer.calledWithBranch, "fix-the-thing")
	}
	if out == "" {
		t.Fatalf("expected non-empty output")
	}
}

func TestSetBranchName_TrimWhitespace(t *testing.T) {
	res, minter := resolverOn(t, "ws-a")
	fakeRenamer := &fakeWorkspaceBranchRenamer{}
	ts := tools.NewToolSet(tools.Deps{
		Resolver:   res,
		Workspaces: fakeRenamer,
	}, "RUN", minter.Mint("RUN"))

	_, err := ts.Call(context.Background(), "set_branch_name", []byte(`{"name":"  fix-the-thing  "}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if fakeRenamer.calledWithBranch != "fix-the-thing" {
		t.Fatalf("renamer not called with trimmed branch name: got %q, want %q", fakeRenamer.calledWithBranch, "fix-the-thing")
	}
}
