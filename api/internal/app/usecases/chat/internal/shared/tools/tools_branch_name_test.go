package tools_test

import (
	"context"
	"strings"
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

type fakeBranchOuter struct{ chatID, workspaceID, branch, by string }

func (f *fakeBranchOuter) RequestBranchOut(_ context.Context, chatID, workspaceID, branch, by string) (bool, error) {
	f.chatID, f.workspaceID, f.branch, f.by = chatID, workspaceID, branch, by
	return false, nil
}

func TestBranchOut_SchedulesTheCallersThreadAsABranchForItsTurnsEnd(t *testing.T) {
	res, minter := resolverOn(t, "ws-a")
	outer := &fakeBranchOuter{}
	ts := tools.NewToolSet(tools.Deps{Resolver: res, Branches: outer}, "RUN", minter.Mint("RUN"))

	got, err := ts.Call(context.Background(), "branch_out", []byte(`{"name":" fix-it "}`))
	if err != nil {
		t.Fatal(err)
	}
	if outer.workspaceID != "ws-a" || outer.branch != "fix-it" || outer.by != "agent" {
		t.Fatalf("requested %+v", outer)
	}
	if !strings.Contains(got, `"fix-it"`) {
		t.Fatalf("the agent is told which branch it will be on: %q", got)
	}
}

// Left unnamed, the branch gets a generated name, and the agent is told how to give it a real one.
func TestBranchOut_AnUnnamedBranchIsGeneratedAndTheAgentToldToRenameIt(t *testing.T) {
	res, minter := resolverOn(t, "ws-a")
	outer := &fakeBranchOuter{}
	ts := tools.NewToolSet(tools.Deps{Resolver: res, Branches: outer}, "RUN", minter.Mint("RUN"))

	got, err := ts.Call(context.Background(), "branch_out", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if outer.branch != "" || !strings.Contains(got, "set_branch_name") {
		t.Fatalf("requested %+v, replied %q", outer, got)
	}
}
