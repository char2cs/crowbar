package main

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// stubTransport answers each path from a queue of canned replies, so the
// create-then-poll flow can be exercised without a daemon. A path that runs out
// of replies keeps serving the last one.
type stubTransport struct {
	replies    map[string][]string
	postStatus int
	postBody   string
	posts      []string
	patches    []string
}

func (s *stubTransport) Get(
	_ context.Context,
	path string,
) (int, []byte, error) {
	queue := s.replies[path]
	if len(queue) == 0 {
		return 404, []byte(`{"success":false,"error":"no stub for ` + path + `"}`), nil
	}
	body := queue[0]
	if len(queue) > 1 {
		s.replies[path] = queue[1:]
	}
	return 200, []byte(body), nil
}

func (s *stubTransport) PostJSON(
	_ context.Context,
	path string,
	_ any,
) (int, []byte, error) {
	s.posts = append(s.posts, path)
	if s.postStatus == 0 {
		return 202, nil, nil
	}
	return s.postStatus, []byte(s.postBody), nil
}

// PatchJSON answers from the same replies queue GET reads from, keyed on
// path: the branch rename is a synchronous mutation whose response the
// caller decodes exactly like a GET's, not a fire-and-forget 202.
func (s *stubTransport) PatchJSON(
	_ context.Context,
	path string,
	_ any,
) (int, []byte, error) {
	s.patches = append(s.patches, path)
	queue := s.replies[path]
	if len(queue) == 0 {
		return 404, []byte(`{"success":false,"error":"no stub for ` + path + `"}`), nil
	}
	body := queue[0]
	if len(queue) > 1 {
		s.replies[path] = queue[1:]
	}
	return 200, []byte(body), nil
}

func TestEnsureProjectReusesAnExistingSeedProject(t *testing.T) {
	wire := &stubTransport{replies: map[string][]string{
		projectsPath: {`{"success":true,"data":[{"id":"p1","name":"Crowbar Seed","path":"/seed"}]}`},
	}}

	got, created, err := ensureProject(context.Background(), &daemon{wire: wire}, "/seed")
	if err != nil {
		t.Fatalf("ensureProject: %v", err)
	}
	if created {
		t.Fatal("an existing seed project must be reused, not re-imported")
	}
	if got.ID != "p1" {
		t.Fatalf("got %+v", got)
	}
	if len(wire.posts) != 0 {
		t.Fatalf("reuse must not POST, got %v", wire.posts)
	}
}

// The import answers 202 and finishes in the background, so the project only
// shows up on a later GET — the tool has to poll rather than trust the 202.
func TestEnsureProjectPollsUntilTheImportLands(t *testing.T) {
	wire := &stubTransport{replies: map[string][]string{
		projectsPath: {
			`{"success":true,"data":[]}`,
			`{"success":true,"data":[]}`,
			`{"success":true,"data":[{"id":"p2","name":"Crowbar Seed","path":"/seed"}]}`,
		},
	}}

	got, created, err := ensureProject(context.Background(), &daemon{wire: wire}, "/seed")
	if err != nil {
		t.Fatalf("ensureProject: %v", err)
	}
	if !created || got.ID != "p2" {
		t.Fatalf("created = %v, got %+v", created, got)
	}
	if len(wire.posts) != 1 || wire.posts[0] != projectsPath {
		t.Fatalf("expected exactly one import POST, got %v", wire.posts)
	}
}

func TestEnsureProjectLeavesUnrelatedProjectsAlone(t *testing.T) {
	wire := &stubTransport{replies: map[string][]string{
		projectsPath: {
			`{"success":true,"data":[{"id":"other","name":"Task18","path":"/elsewhere"}]}`,
			`{"success":true,"data":[{"id":"other","name":"Task18","path":"/elsewhere"},` +
				`{"id":"p3","name":"Crowbar Seed","path":"/seed"}]}`,
		},
	}}

	got, _, err := ensureProject(context.Background(), &daemon{wire: wire}, "/seed")
	if err != nil {
		t.Fatalf("ensureProject: %v", err)
	}
	if got.ID != "p3" {
		t.Fatalf("got %+v, want the seed project", got)
	}
}

func TestEnsureRepoReusesAnExistingSeedRepo(t *testing.T) {
	path := projectsPath + "/p1/repos"
	wire := &stubTransport{replies: map[string][]string{
		path: {`{"success":true,"data":[{"id":"r1","projectId":"p1","name":"checkout","defaultBranch":"main"}]}`},
	}}

	got, created, err := ensureRepo(context.Background(), &daemon{wire: wire}, "p1", "/seed/checkout")
	if err != nil {
		t.Fatalf("ensureRepo: %v", err)
	}
	if created || got.ID != "r1" || got.DefaultBranch != seedBaseBranch {
		t.Fatalf("created = %v, got %+v", created, got)
	}
	if len(wire.posts) != 0 {
		t.Fatalf("reuse must not POST, got %v", wire.posts)
	}
}

// The provider list must be read before the fork POST: forking always starts
// a runner (own_worktree.go has no empty-providerID skip the import path
// gives itself), so ensureFeatureChat needs an enabled provider to hand it.
func TestEnsureFeatureChatForksWithAnEnabledProvider(t *testing.T) {
	repo := repoDTO{ID: "r1", ProjectID: "p1", DefaultBranch: seedBaseBranch}
	path := chatsPath("p1", "r1")
	detail := chatDetailPath("p1", "r1", "chat1")
	wire := &stubTransport{
		replies: map[string][]string{
			path + "/providers": {`{"success":true,"data":[{"id":"claude","enabled":true}]}`},
			path: {
				`{"success":true,"data":[]}`,
				`{"success":true,"data":[{"id":"chat1","workspaceId":"ws1",` +
					`"worktree":{"branch":"feature/pricing-rounding","localPath":"/wt/seed"}}]}`,
			},
			detail + "/branch": {`{"success":true,"data":{"id":"chat1"}}`},
		},
		postStatus: 201,
		postBody:   `{"success":true,"data":{"id":"chat1"}}`,
	}

	got, created, err := ensureFeatureChat(context.Background(), &daemon{wire: wire}, repo, "base")
	if err != nil {
		t.Fatalf("ensureFeatureChat: %v", err)
	}
	if !created || got.ID != "chat1" || got.WorkspaceID != "ws1" {
		t.Fatalf("created = %v, got %+v", created, got)
	}
	if got.Worktree == nil || got.Worktree.Branch != seedFeatureBranch || got.Worktree.LocalPath != "/wt/seed" {
		t.Fatalf("worktree = %+v", got.Worktree)
	}
	if len(wire.posts) != 2 || wire.posts[0] != path || wire.posts[1] != detail+"/stop" {
		t.Fatalf("expected a create POST then a stop POST, got %v", wire.posts)
	}
	if len(wire.patches) != 1 || wire.patches[0] != detail+"/branch" {
		t.Fatalf("expected exactly one branch-rename PATCH, got %v", wire.patches)
	}
}

func TestEnsureFeatureChatReusesAnExistingFork(t *testing.T) {
	repo := repoDTO{ID: "r1", ProjectID: "p1", DefaultBranch: seedBaseBranch}
	path := chatsPath("p1", "r1")
	wire := &stubTransport{replies: map[string][]string{
		path: {`{"success":true,"data":[{"id":"chat1","workspaceId":"ws1",` +
			`"worktree":{"branch":"feature/pricing-rounding","localPath":"/wt/seed"}}]}`},
	}}

	got, created, err := ensureFeatureChat(context.Background(), &daemon{wire: wire}, repo, "base")
	if err != nil {
		t.Fatalf("ensureFeatureChat: %v", err)
	}
	if created || got.ID != "chat1" {
		t.Fatalf("created = %v, got %+v", created, got)
	}
	if len(wire.posts) != 0 || len(wire.patches) != 0 {
		t.Fatalf("reuse must not mutate anything, got posts=%v patches=%v", wire.posts, wire.patches)
	}
}

func TestEnsureFeatureChatFailsClearlyWithNoEnabledProvider(t *testing.T) {
	repo := repoDTO{ID: "r1", ProjectID: "p1", DefaultBranch: seedBaseBranch}
	path := chatsPath("p1", "r1")
	wire := &stubTransport{replies: map[string][]string{
		path:                {`{"success":true,"data":[]}`},
		path + "/providers": {`{"success":true,"data":[{"id":"claude","enabled":false}]}`},
	}}

	_, _, err := ensureFeatureChat(context.Background(), &daemon{wire: wire}, repo, "base")
	if err == nil {
		t.Fatal("expected a clear error when no provider is enabled")
	}
	if !strings.Contains(err.Error(), "no enabled provider") {
		t.Fatalf("error = %v", err)
	}
	if len(wire.posts) != 0 {
		t.Fatalf("must not attempt a create with no provider to run it, got %v", wire.posts)
	}
}

func TestEnsureThreadsSkipsAlreadySeededComments(t *testing.T) {
	sc := scope{projectID: "p1", repoID: "r1", workspaceID: "ws"}
	first, err := lineOf(branchPricingSource, seedThreads()[0].anchor)
	if err != nil {
		t.Fatalf("lineOf: %v", err)
	}
	wire := &stubTransport{
		replies:    map[string][]string{sc.path("/threads"): {threadsListJSON(first)}},
		postStatus: 201,
		postBody:   `{"success":true,"data":{"id":"t2","filePath":"` + pricingPath + `","line":50}}`,
	}

	created, err := ensureThreads(context.Background(), &daemon{wire: wire}, sc, "Ada Lovelace")
	if err != nil {
		t.Fatalf("ensureThreads: %v", err)
	}
	if created != 1 {
		t.Fatalf("created %d threads, want only the missing one", created)
	}
	if len(wire.posts) != 1 {
		t.Fatalf("expected one POST, got %v", wire.posts)
	}
}

func TestResolveAuthorFallsBackWhenIdentityIsUnavailable(t *testing.T) {
	got := resolveAuthor(context.Background(), &daemon{wire: &stubTransport{}}, "chat1")

	if got != fallbackReviewer {
		t.Fatalf("author = %q, want the fallback", got)
	}
}

func TestResolveAuthorPrefersTheDisplayName(t *testing.T) {
	wire := &stubTransport{replies: map[string][]string{
		flatChatPath("chat1", "/identity"): {`{"success":true,"data":{"login":"ada","displayName":"Ada Lovelace"}}`},
	}}

	got := resolveAuthor(context.Background(), &daemon{wire: wire}, "chat1")

	if got != "Ada Lovelace" {
		t.Fatalf("author = %q", got)
	}
}

func TestPostAcceptedRejectsARefusedMutation(t *testing.T) {
	wire := &refusingTransport{}

	err := (&daemon{wire: wire}).postAccepted(context.Background(), "create workspace", "/v0/x", nil)

	if err == nil {
		t.Fatal("a 409 must not be treated as an accepted mutation")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Fatalf("error should name the status, got %v", err)
	}
}

type refusingTransport struct{}

func (refusingTransport) Get(
	_ context.Context,
	_ string,
) (int, []byte, error) {
	return 409, nil, nil
}

func (refusingTransport) PostJSON(
	_ context.Context,
	_ string,
	_ any,
) (int, []byte, error) {
	return 409, []byte(`{"success":false,"error":"a workspace already exists for this branch"}`), nil
}

func (refusingTransport) PatchJSON(
	_ context.Context,
	_ string,
	_ any,
) (int, []byte, error) {
	return 409, []byte(`{"success":false,"error":"a workspace already exists for this branch"}`), nil
}

func threadsListJSON(
	line int,
) string {
	return `{"success":true,"data":[{"id":"t1","filePath":"` + pricingPath +
		`","line":` + strconv.Itoa(line) + `,"author":"Ada Lovelace"}]}`
}
