package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter"
	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	"github.com/char2cs/crowbar/api/internal/perf"
)

// manyThreads builds n unresolved threads with ids "t-1".."t-n", so a test can
// name the exact thread it expects at each edge of a page rather than counting
// rows and hoping the count lines up with the right ones.
func manyThreads(n int) []domain.ReviewThread {
	out := make([]domain.ReviewThread, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, domain.ReviewThread{
			ID: fmt.Sprintf("t-%d", i), WsID: "ws-a",
			FilePath: fmt.Sprintf("f%d.go", i), StartLine: 1, EndLine: 1,
			Side: domain.ReviewSideRight, Status: domain.ReviewThreadStatusOpen,
			Messages: []domain.ReviewMessage{{ID: "m1", Author: "mateo", Body: "look at this"}},
		})
	}
	return out
}

func manyFiles(n int) []gitdomain.ReviewFileSummary {
	out := make([]gitdomain.ReviewFileSummary, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, gitdomain.ReviewFileSummary{
			Path:      fmt.Sprintf("src/f%d.go", i),
			Status:    gitdomain.GitFileStatusModified,
			Additions: 1, Deletions: 1,
		})
	}
	return out
}

// threadRows counts rendered thread anchor rows: the unindented lines that are
// neither the pagination note nor the column header. Counting rows is what
// proves the CAP, as opposed to merely proving that some particular thread is
// missing.
func threadRows(out string) int {
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		switch {
		case line == "", strings.HasPrefix(line, " "):
		case strings.HasPrefix(line, "Showing"), strings.HasPrefix(line, "No "):
		case strings.HasPrefix(line, "id  file:lines"):
		default:
			n++
		}
	}
	return n
}

func fileRows(out string) int {
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, string(gitdomain.GitFileStatusModified)+"  +") {
			n++
		}
	}
	return n
}

// longBody builds a body of n identical characters, so a test can name the exact
// prefix it expects to survive a cut and the exact count it expects reported.
func longBody(n int) string {
	return strings.Repeat("x", n)
}

// TestToolSet_RecordsEveryCallIncludingUnauthorized proves the deliberate
// attribution choice documented on ToolSet.Call: Resolve fails before name is
// checked against the registered tools, and the call is still counted under
// the literal name the caller asked for — a rejected attempt at a specific
// tool is exactly the datum this counter exists to surface.
func TestToolSet_RecordsEveryCallIncludingUnauthorized(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{}, stubWorkspaces{all: tree()})

	ts := tools.NewToolSet(tools.Deps{
		Resolver: res,
		Chats:    &spyRenamer{},
		Metrics:  metrics,
	}, "RUN", "forged-token")

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.ErrorIs(t, err, tools.ErrUnauthorized)

	stat := metrics.Snapshot()["set_chat_title"]
	require.Equal(t, 1, stat.Calls)
	require.Equal(t, 1, stat.Failures)
}

// TestToolSet_RecordsSuccessfulCalls is the counterpart to the unauthorized
// case above: a call that actually reaches its handler and succeeds must be
// counted as a call with zero failures, not merely as a non-error return.
func TestToolSet_RecordsSuccessfulCalls(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	tok := minter.Mint("RUN")

	ts := tools.NewToolSet(tools.Deps{
		Resolver: res,
		Chats:    &spyRenamer{},
		Metrics:  metrics,
	}, "RUN", tok)

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"Refactor auth"}`))
	require.NoError(t, err)

	stat := metrics.Snapshot()["set_chat_title"]
	require.Equal(t, 1, stat.Calls)
	require.Equal(t, 0, stat.Failures)
}

// TestToolSet_FoldsUnknownToolNamesIntoOneBucket bounds the counter map. The
// name reaches Record straight off the wire, before it is checked against the
// registered tools, and the caller is a MODEL — so a hallucinating agent could
// otherwise add one map entry per invented name and grow it for the whole life
// of the daemon. Every unregistered name lands in a single bucket; the real
// tools keep their own attribution.
func TestToolSet_FoldsUnknownToolNamesIntoOneBucket(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})

	ts := tools.NewToolSet(tools.Deps{
		Resolver: res,
		Chats:    &spyRenamer{},
		Metrics:  metrics,
	}, "RUN", minter.Mint("RUN"))

	for _, invented := range []string{"rm_rf", "list_secrets", "get_review_scop", "🙂"} {
		_, err := ts.Call(context.Background(), invented, json.RawMessage(`{}`))
		require.Error(t, err)
	}
	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.NoError(t, err)

	snap := metrics.Snapshot()
	require.Equal(t, tools.ToolStat{Calls: 4, Failures: 4}, snap[tools.UnknownToolMetric])
	require.Equal(t, tools.ToolStat{Calls: 1, Failures: 0}, snap["set_chat_title"])
	require.Len(t, snap, 2, "four invented names must not add four keys to the counter map")
}

// A name is folded by whether THIS ToolSet registered it, not by a hardcoded
// list: a tool suppressed because its dependency is missing is genuinely not a
// tool this caller has, so an attempt at it is not something to attribute
// per-name either.
func TestToolSet_FoldsAToolThisSetDidNotRegister(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})

	// Chats only: list_review_threads is a real tool name, but not on this set.
	ts := tools.NewToolSet(tools.Deps{
		Resolver: res,
		Chats:    &spyRenamer{},
		Metrics:  metrics,
	}, "RUN", minter.Mint("RUN"))

	_, err = ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{}`))
	require.Error(t, err)

	snap := metrics.Snapshot()
	require.Equal(t, tools.ToolStat{Calls: 1, Failures: 1}, snap[tools.UnknownToolMetric])
	require.NotContains(t, snap, "list_review_threads")
}

// TestToolSet_NilMetricsStillRegistersAllEightTools is the fail-open guard:
// every other Deps port suppresses its tool group when nil, but Metrics must
// not — losing the call counters is never a reason to lose a capability. The
// shared toolsetOn fixture never sets Metrics, so this also doubles as proof
// that a production Deps left without Metrics wired keeps working.
func TestToolSet_NilMetricsStillRegistersAllEightTools(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	require.Len(t, ts.Tools(), 8)

	out, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.NoError(t, err)
	require.Contains(t, out, "x")
}

// perfScatterEvery is how many lines apart a changed line is placed. Git's
// default context is 3 lines, so changes 10 apart never coalesce and the hunk
// count is exactly the changed-line count — which is what lets the fixture claim
// a hunk total rather than estimate one.
const perfScatterEvery = 10

// perfRunnerID names the single runner every measured call authenticates as.
const perfRunnerID = "perf-runner"

// countingWorkspaces counts the two workspace reads the tool surface makes and
// delegates everything else to the real repository. It is embedded rather than
// reimplemented because only Get and List are under measurement and the
// aggregate fold behind them — the actual cost — must stay real.
type countingWorkspaces struct {
	workspace.Workspace
	gets  atomic.Int64
	lists atomic.Int64
}

func (c *countingWorkspaces) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	c.gets.Add(1)
	return c.Workspace.Get(ctx, id)
}

func (c *countingWorkspaces) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	c.lists.Add(1)
	return c.Workspace.List(ctx)
}

func (c *countingWorkspaces) reset() {
	c.gets.Store(0)
	c.lists.Store(0)
}

// countingReconciler stands in for the production reconcile-on-open trigger.
// It counts and does nothing else: the point is to show how many background
// reconciles one tool call fires, and running the real git+provider
// re-derivation would put an unrelated cost inside the window being timed.
type countingReconciler struct {
	opens atomic.Int64
}

func (c *countingReconciler) OnOpen(
	_ context.Context,
	_ string,
) {
	c.opens.Add(1)
}

// countingChats adapts the chat repository to tools.ChatReader — the same
// name-only adaptation the production container does — and counts both reads.
//
// byWs survives Task A3 even though the tool surface no longer calls
// ListByWorkspace: the whole claim of A3 is that V per-workspace reads became
// one whole-table read, and a counter that only exists on the winning side
// cannot show the losing one going to zero.
type countingChats struct {
	chats agentchat.EventStore
	gets  atomic.Int64
	byWs  atomic.Int64
	lists atomic.Int64
}

func (c *countingChats) Get(
	ctx context.Context,
	chatID string,
) (domain.Chat, error) {
	c.gets.Add(1)
	return c.chats.GetChat(ctx, chatID)
}

func (c *countingChats) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	c.byWs.Add(1)
	return c.chats.ListByWorkspace(ctx, wsID)
}

func (c *countingChats) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	c.lists.Add(1)
	return c.chats.ListChats(ctx)
}

func (c *countingChats) reset() {
	c.gets.Store(0)
	c.byWs.Store(0)
	c.lists.Store(0)
}

// perfRenamer is the ChatRenamer set_chat_title writes through. It does nothing
// on purpose: the tool is measured for the cost of GETTING to a handler — the
// resolve every MCP call pays — and a real rename would put an unrelated write
// inside that window.
type perfRenamer struct{}

func (perfRenamer) RenameByRunner(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) error {
	return nil
}

// queryCounter counts SELECTs and rows materialised on one GORM handle. It is
// registered on the chat read model because ListByWorkspace filters in Go after
// reading every row: the call count alone says how often the table was touched,
// and only the row count says how much was read to answer with a fraction of it.
type queryCounter struct {
	queries atomic.Int64
	rows    atomic.Int64
}

func (q *queryCounter) record(
	tx *gormdb.DB,
) {
	q.queries.Add(1)
	q.rows.Add(tx.RowsAffected)
}

func (q *queryCounter) reset() {
	q.queries.Store(0)
	q.rows.Store(0)
}

// perfStack is the production wiring of the agent tool surface over real
// repositories, a real git engine and a real repo on disk, with the counting
// seams spliced in.
type perfStack struct {
	tools       *tools.ToolSet
	workspaces  *countingWorkspaces
	chats       *countingChats
	chatQueries *queryCounter
	reconciler  *countingReconciler
	chatStore   agentchat.EventStore
	runners     agentrunner.EventStore
	repos       store.Store[domain.Repository, string]
	repoPath    string
	featurePath string
	baseBranch  string
}

func (s *perfStack) resetCounters() {
	s.workspaces.reset()
	s.chats.reset()
	s.chatQueries.reset()
	s.reconciler.opens.Store(0)
	perf.Reset()
}

// newPerfStack builds the whole stack minus the git fixture, which the two
// benchmark families seed differently.
func newPerfStack(
	b *testing.B,
) *perfStack {
	b.Helper()

	adapters, err := adapter.New(adapter.WithHomeDir(b.TempDir()))
	require.NoError(b, err)
	b.Cleanup(func() { _ = adapters.Close() })

	reconciler := &countingReconciler{}
	workspaces := newPerfWorkspaces(b, adapters, reconciler)
	threads := newPerfThreads(b, adapters)

	repoStore, err := storesqlite.NewFromDB[domain.Repository, string](adapters.GlobalView())
	require.NoError(b, err)

	chatQueries := &queryCounter{}
	require.NoError(b, adapters.AgentChatReadDB().
		Callback().Query().After("gorm:query").
		Register("agenttools_perf:count", chatQueries.record))

	chatStore := newPerfChatStore(b, adapters)
	runners := newPerfRunnerStore(b, adapters)
	chats := &countingChats{chats: chatStore}

	minter, err := tools.NewTokenMinter()
	require.NoError(b, err)

	review := branchreview.New(
		workspaces,
		threads,
		repoStore,
		enginegit.New(),
		func() time.Time { return time.Unix(1_000_000, 0).UTC() },
	)

	tools := tools.NewToolSet(tools.Deps{
		Resolver:  tools.NewResolver(minter, runners, chats, workspaces),
		Review:    review,
		Chats:     perfRenamer{},
		ChatReads: chats,
		Metrics:   tools.NewMetrics(),
	}, perfRunnerID, minter.Mint(perfRunnerID))

	return &perfStack{
		tools:       tools,
		workspaces:  workspaces,
		chats:       chats,
		chatQueries: chatQueries,
		reconciler:  reconciler,
		chatStore:   chatStore,
		runners:     runners,
		repos:       repoStore,
	}
}

func newPerfWorkspaces(
	b *testing.B,
	adapters *adapter.Container,
	reconciler workspace.ReconcileOnOpener,
) *countingWorkspaces {
	b.Helper()
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(adapters.WorkspaceES()).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	paths, err := wspaths.NewWorkspacePaths(adapters.GlobalView())
	require.NoError(b, err)
	repo, err := workspace.New(
		ax,
		adapters.WorkspaceES(),
		adapters.WorkspaceView(),
		paths,
		workspace.WithReconciler(reconciler),
	)
	require.NoError(b, err)
	return &countingWorkspaces{Workspace: repo}
}

func newPerfThreads(
	b *testing.B,
	adapters *adapter.Container,
) reviewthread.ReviewThread {
	b.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(b, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	threads, err := reviewthread.New(ax, es, adapters.ReviewThreadView(), func(domain.ReviewThread) {})
	require.NoError(b, err)
	return threads
}

func newPerfChatStore(
	b *testing.B,
	adapters *adapter.Container,
) agentchat.EventStore {
	b.Helper()
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(adapters.AgentChatES()).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	store, err := agentchat.NewEventSourced(
		ax,
		adapters.AgentChatES(),
		adapters.AgentChatReadDB(),
		func(agentchat.ChatEvent) {},
	)
	require.NoError(b, err)
	return store
}

func newPerfRunnerStore(
	b *testing.B,
	adapters *adapter.Container,
) agentrunner.EventStore {
	b.Helper()
	ax, err := asynx.New[agents.Runner]().
		WithEventStore(adapters.AgentRunnerES()).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	store, err := agentrunner.NewEventSourced(
		ax,
		adapters.AgentRunnerES(),
		adapters.AgentRunnerReadDB(),
		func(agentrunner.RunnerEvent) {},
	)
	require.NoError(b, err)
	return store
}

// perfDiffSpec is the shape of one review fixture. hunks is per file and is
// exact, not approximate: perfScatterEvery keeps every changed line further
// apart than git's context window, so no two changes ever merge.
type perfDiffSpec struct {
	name  string
	files int
	hunks int
}

func (s perfDiffSpec) linesPerFile() int {
	return s.hunks*perfScatterEvery + perfScatterEvery/2
}

func (s perfDiffSpec) totalHunks() int {
	return s.files * s.hunks
}

// perfLine is a pure function of (salt, index): a rerun produces byte-identical
// content, so two measurements of the same spec are comparable.
func perfLine(
	salt string,
	i int,
) string {
	return fmt.Sprintf("%s line %d token %d\n", salt, i, (i*2654435761+len(salt))%1000003)
}

func perfText(
	salt string,
	lines int,
) string {
	var b strings.Builder
	b.Grow(lines * 40)
	for i := 1; i <= lines; i++ {
		b.WriteString(perfLine(salt, i))
	}
	return b.String()
}

// perfTextChanged rewrites every perfScatterEvery-th line, leaving the rest
// byte-identical, so the file's diff is exactly lines/perfScatterEvery hunks.
func perfTextChanged(
	baseSalt string,
	headSalt string,
	lines int,
) string {
	var b strings.Builder
	b.Grow(lines * 40)
	for i := 1; i <= lines; i++ {
		salt := baseSalt
		if i%perfScatterEvery == 0 {
			salt = headSalt
		}
		b.WriteString(perfLine(salt, i))
	}
	return b.String()
}

func perfGit(
	b *testing.B,
	dir string,
	args ...string,
) string {
	b.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		cmd.Environ(),
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t",
		// The fixture's own setup must never land in the trace file the
		// measurement reads back.
		"GIT_TRACE=0",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(b, err, string(out))
	return string(out)
}

func perfWriteFile(
	b *testing.B,
	root string,
	rel string,
	content string,
) {
	b.Helper()
	full := filepath.Join(root, rel)
	require.NoError(b, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(b, os.WriteFile(full, []byte(content), 0o600))
}

func perfTrimNewline(
	s string,
) string {
	return strings.TrimRight(s, "\r\n")
}

// seedReviewFixture builds a real repo whose feature branch carries spec's diff,
// and an `origin` remote carrying the base branch.
//
// The remote is not decoration. resolveDiffRef probes BOTH origin/<base> and
// <base> and only compares them when both resolve, so a fixture with no origin
// would run two merge-base spawns per resolution where production runs three —
// understating by a third the very duplication this baseline exists to size.
func seedReviewFixture(
	b *testing.B,
	s *perfStack,
	spec perfDiffSpec,
) {
	b.Helper()

	repoPath := b.TempDir()
	perfGit(b, repoPath, "init")
	perfGit(b, repoPath, "config", "user.email", "t@t")
	perfGit(b, repoPath, "config", "user.name", "t")
	perfGit(b, repoPath, "commit", "--allow-empty", "-m", "init")
	baseBranch := perfTrimNewline(perfGit(b, repoPath, "rev-parse", "--abbrev-ref", "HEAD"))

	for i := range spec.files {
		perfWriteFile(b, repoPath, perfRelPath(i),
			perfText(fmt.Sprintf("base%d", i), spec.linesPerFile()))
	}
	perfGit(b, repoPath, "add", "-A")
	perfGit(b, repoPath, "commit", "-m", "perf: base tree")

	origin := b.TempDir()
	perfGit(b, origin, "init", "--bare")
	perfGit(b, repoPath, "remote", "add", "origin", origin)
	perfGit(b, repoPath, "push", "-u", "origin", baseBranch)

	featurePath := filepath.Join(b.TempDir(), "feature-wt")
	perfGit(b, repoPath, "worktree", "add", "-b", "feature/perf-review", featurePath)
	for i := range spec.files {
		perfWriteFile(b, featurePath, perfRelPath(i), perfTextChanged(
			fmt.Sprintf("base%d", i), fmt.Sprintf("head%d", i), spec.linesPerFile(),
		))
	}
	perfGit(b, featurePath, "add", "-A")
	perfGit(b, featurePath, "commit", "-m", "perf: feature tree")

	s.repoPath = repoPath
	s.featurePath = featurePath
	s.baseBranch = baseBranch
}

func perfRelPath(
	i int,
) string {
	return fmt.Sprintf("src/pkg%d/file%d.ts", i%8, i)
}

const (
	perfProjectID = "perf-project"
	perfRepoID    = "perf-repo"
)

func perfNow() time.Time {
	return time.Unix(1_000_000, 0).UTC()
}

// seedReviewWorkspace persists the repository row and the feature workspace the
// review tools resolve through, and places one runner on one chat inside it.
func seedReviewWorkspace(
	b *testing.B,
	s *perfStack,
) {
	b.Helper()
	ctx := context.Background()

	require.NoError(b, s.repos.Save(ctx, domain.Repository{
		ID:            perfRepoID,
		ProjectID:     perfProjectID,
		Name:          perfRepoID,
		Path:          s.repoPath,
		DefaultBranch: s.baseBranch,
	}))
	_, err := s.workspaces.Create(ctx, workspace.CreateInput{
		ID:           "perf-ws",
		RepoID:       perfRepoID,
		ProjectID:    perfProjectID,
		Branch:       "feature/perf-review",
		WorktreePath: s.featurePath,
		Kind:         domain.WorkspaceKindGit,
	}, perfNow())
	require.NoError(b, err)
	seedRunnerOn(b, s, "perf-ws")
}

// seedContextTree builds the hierarchy list_workspaces walks: one default
// workspace (whose caller therefore sees the whole repo) plus children, each
// carrying chats. It touches no git — the tool reads only the workspace tree and
// the chat table.
func seedContextTree(
	b *testing.B,
	s *perfStack,
	workspaces int,
	chatsPer int,
) {
	b.Helper()
	ctx := context.Background()

	for i := range workspaces {
		_, err := s.workspaces.Create(ctx, workspace.CreateInput{
			ID:        fmt.Sprintf("perf-ws-%d", i),
			RepoID:    perfRepoID,
			ProjectID: perfProjectID,
			Branch:    fmt.Sprintf("feature/perf-%d", i),
			Kind:      domain.WorkspaceKindGit,
			IsDefault: i == 0,
		}, perfNow())
		require.NoError(b, err)
		for j := range chatsPer {
			_, err := s.chatStore.Create(ctx, agentchat.CreateInput{
				ID:          fmt.Sprintf("perf-chat-%d-%d", i, j),
				WorkspaceID: fmt.Sprintf("perf-ws-%d", i),
				Now:         perfNow(),
			})
			require.NoError(b, err)
		}
	}
	seedRunnerOn(b, s, "perf-ws-0")
}

func seedRunnerOn(
	b *testing.B,
	s *perfStack,
	wsID string,
) {
	b.Helper()
	ctx := context.Background()

	chatID := wsID + "-caller-chat"
	_, err := s.chatStore.Create(ctx, agentchat.CreateInput{
		ID:          chatID,
		WorkspaceID: wsID,
		Now:         perfNow(),
	})
	require.NoError(b, err)
	_, err = s.runners.Start(ctx, agentrunner.StartInput{
		RunnerID:        perfRunnerID,
		WorkspaceID:     wsID,
		ProviderID:      "perf-provider",
		TerminalSession: "perf-term",
		ChatID:          chatID,
		Now:             perfNow(),
	})
	require.NoError(b, err)
}

// perfBucket is one perf-ring sample name's tally: how many samples landed under
// it and how much wall time they account for. The pair is what separates "this
// subcommand is called too often" from "this subcommand is slow" — two findings
// with opposite fixes.
type perfBucket struct {
	n       int
	totalMS float64
}

// perfCounts is one tool call's cost, in units that survive a warm cache.
type perfCounts struct {
	wall       time.Duration
	calls      int
	git        map[string]*perfBucket
	gitTotal   int
	gitMS      float64
	locks      int
	wsGet      int
	wsList     int
	chatByWs   int
	chatList   int
	chatGet    int
	onOpen     int
	sqlQueries int
	sqlRows    int
}

// perCall divides by the number of calls the window covered, so a cold
// single-shot and a warm b.N loop read on the same scale.
func (c perfCounts) perCall(
	n int,
) float64 {
	return float64(n) / float64(c.calls)
}

func (c perfCounts) log(
	b *testing.B,
	label string,
) {
	b.Helper()
	b.Logf(
		"%s: wall=%.1fms/call git=%.1f spawns (%.1fms) locks=%.1f "+
			"ws.Get=%.1f ws.List=%.1f chat.ListByWorkspace=%.1f chat.ListChats=%.1f chat.Get=%.1f "+
			"reconcile.OnOpen=%.1f chatSQL.queries=%.1f chatSQL.rows=%.1f",
		label,
		float64(c.wall.Microseconds())/1000/float64(c.calls),
		c.perCall(c.gitTotal),
		c.gitMS/float64(c.calls),
		c.perCall(c.locks),
		c.perCall(c.wsGet),
		c.perCall(c.wsList),
		c.perCall(c.chatByWs),
		c.perCall(c.chatList),
		c.perCall(c.chatGet),
		c.perCall(c.onOpen),
		c.perCall(c.sqlQueries),
		c.perCall(c.sqlRows),
	)
	for _, name := range sortedBucketKeys(c.git) {
		bucket := c.git[name]
		b.Logf(
			"    %-22s %.1f spawns/call  %6.1fms/call  %5.1fms/spawn",
			name,
			c.perCall(bucket.n),
			bucket.totalMS/float64(c.calls),
			bucket.totalMS/float64(bucket.n),
		)
	}
}

func sortedKeys(
	m map[string]int,
) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedBucketKeys(
	m map[string]*perfBucket,
) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// callTool invokes one tool exactly as the MCP relay does — through ToolSet.Call,
// so the Resolve that every MCP call pays for is inside the measured window.
func callTool(
	b *testing.B,
	s *perfStack,
	name string,
	args json.RawMessage,
) string {
	b.Helper()
	out, err := s.tools.Call(context.Background(), name, args)
	require.NoError(b, err)
	require.NotEmpty(b, out)
	return out
}

// measureCold times the FIRST call against a fresh stack. It is reported
// separately from the warm loop because an agent calling get_review_scope once
// per review pays exactly this, and the caches underneath it (the engine's
// git-common-dir map, the committed-diff summary cache, git's own page cache)
// make every later call a different measurement.
func measureCold(
	b *testing.B,
	s *perfStack,
	name string,
	args json.RawMessage,
) perfCounts {
	b.Helper()
	s.resetCounters()
	perf.SetEnabled(true)
	start := time.Now()
	callTool(b, s, name, args)
	elapsed := time.Since(start)
	perf.SetEnabled(false)
	return collect(s, elapsed, 1)
}

// measureWarm runs the b.N loop with the ring armed and returns the steady-state
// cost — what an agent polling the tool repeatedly would see.
func measureWarm(
	b *testing.B,
	s *perfStack,
	name string,
	args json.RawMessage,
) perfCounts {
	b.Helper()
	callTool(b, s, name, args)
	s.resetCounters()
	perf.SetEnabled(true)
	b.ResetTimer()
	start := time.Now()
	for range b.N {
		callTool(b, s, name, args)
	}
	elapsed := time.Since(start)
	b.StopTimer()
	perf.SetEnabled(false)
	return collect(s, elapsed, b.N)
}

// collect reads every instrument back into one record. Lock samples are counted
// separately from spawns: "lock.read.hold" is one sample per repo-lock
// acquisition, not a subprocess, and folding it into the spawn total would
// inflate it.
func collect(
	s *perfStack,
	elapsed time.Duration,
	calls int,
) perfCounts {
	out := perfCounts{wall: elapsed, calls: calls, git: map[string]*perfBucket{}}
	for _, sample := range perf.Snapshot() {
		if !strings.HasPrefix(sample.Name, "git.") {
			out.locks++
			continue
		}
		bucket, ok := out.git[sample.Name]
		if !ok {
			bucket = &perfBucket{}
			out.git[sample.Name] = bucket
		}
		bucket.n++
		bucket.totalMS += sample.DurationMS
		out.gitTotal++
		out.gitMS += sample.DurationMS
	}
	out.wsGet = int(s.workspaces.gets.Load())
	out.wsList = int(s.workspaces.lists.Load())
	out.chatByWs = int(s.chats.byWs.Load())
	out.chatList = int(s.chats.lists.Load())
	out.chatGet = int(s.chats.gets.Load())
	out.onOpen = int(s.reconciler.opens.Load())
	out.sqlQueries = int(s.chatQueries.queries.Load())
	out.sqlRows = int(s.chatQueries.rows.Load())
	return out
}

// logGitArgv runs one more call with GIT_TRACE pointed at a file and logs the
// full argv of every subprocess, in order. The perf ring buckets by subcommand,
// so it can say "6 merge-base spawns" but not that they resolve the same two
// refs three times over; that claim needs the arguments, and git is the only
// thing that can report them without a production hook.
func logGitArgv(
	b *testing.B,
	s *perfStack,
	name string,
) {
	b.Helper()
	trace := filepath.Join(b.TempDir(), "git-trace.log")
	b.Setenv("GIT_TRACE", trace)
	callTool(b, s, name, nil)
	b.Setenv("GIT_TRACE", "0")

	data, err := os.ReadFile(trace)
	if os.IsNotExist(err) {
		b.Logf("  argv: no git subprocesses")
		return
	}
	require.NoError(b, err)
	counted := map[string]int{}
	for _, line := range strings.Split(string(data), "\n") {
		_, argv, found := strings.Cut(line, "trace: built-in: ")
		if !found {
			continue
		}
		counted[argv]++
	}
	for _, argv := range sortedKeys(counted) {
		b.Logf("    %2dx  %s", counted[argv], argv)
	}
}

// perfReviewSpecs answer "how much of this is fixed overhead?" by bracketing it.
// The middle one is a realistic branch; the small one is the smallest diff that
// still exists, so everything the two share is fixed cost. The large one exists
// because a pair of points that agree proves only that the cost is flat BETWEEN
// them — it takes a third, an order of magnitude out, to find where the flat
// part ends and diff-proportional work begins.
var perfReviewSpecs = []perfDiffSpec{
	{name: "40files_200hunks", files: 40, hunks: 5},
	{name: "1file_1hunk", files: 1, hunks: 1},
	{name: "200files_10000hunks", files: 200, hunks: 50},
}

// BenchmarkPerf_GetReviewScope measures the tool an agent calls before every
// review.
//
// The baseline it was written against was GetBase followed by GetFiles(""),
// each resolving the same diff ref independently: 9 warm spawns, 6 of them
// merge-base, of which 4 were byte-identical duplicates and 2 compared a sha
// with itself. Task A2 collapsed that to a single GetScope resolution.
func BenchmarkPerf_GetReviewScope(
	b *testing.B,
) {
	for _, spec := range perfReviewSpecs {
		b.Run(spec.name, func(b *testing.B) {
			s := newPerfStack(b)
			seedReviewFixture(b, s, spec)
			seedReviewWorkspace(b, s)
			b.Logf("fixture: %d files, %d hunks", spec.files, spec.totalHunks())

			cold := measureCold(b, s, "get_review_scope", nil)
			warm := measureWarm(b, s, "get_review_scope", nil)
			cold.log(b, "cold")
			warm.log(b, "warm")
			logGitArgv(b, s, "get_review_scope")
		})
	}
}

// perfContextShape sizes the O(V·C) surface of list_workspaces: V visible
// workspaces, C chats in each.
type perfContextShape struct {
	name       string
	workspaces int
	chatsPer   int
}

var perfContextShapes = []perfContextShape{
	{name: "12ws_5chats", workspaces: 12, chatsPer: 5},
	{name: "1ws_1chat", workspaces: 1, chatsPer: 1},
}

// BenchmarkPerf_ListWorkspaces measures the tool that loops ListByWorkspace over
// every visible workspace, each call a full scan of the whole chat table.
func BenchmarkPerf_ListWorkspaces(
	b *testing.B,
) {
	for _, shape := range perfContextShapes {
		b.Run(shape.name, func(b *testing.B) {
			s := newPerfStack(b)
			seedContextTree(b, s, shape.workspaces, shape.chatsPer)
			b.Logf("fixture: %d workspaces, %d chats each", shape.workspaces, shape.chatsPer)

			cold := measureCold(b, s, "list_workspaces", nil)
			warm := measureWarm(b, s, "list_workspaces", nil)
			cold.log(b, "cold")
			warm.log(b, "warm")
		})
	}
}

// BenchmarkPerf_SetChatTitle measures what an MCP call costs when the tool it
// names needs NOTHING but the caller's own runner.
//
// It is here because the other two benchmarks cannot show the cost this one
// isolates. get_review_scope spends ~50ms in five git subprocesses, which buries
// anything measured in microseconds; list_workspaces genuinely wants the
// workspace tree, so reading it there is work, not waste. set_chat_title renames
// the chat its runner is on and never looks at another workspace — so every
// microsecond it spends on the workspace tree is pure overhead, paid on every
// call an agent makes, and it is the only place that overhead is visible on its
// own.
//
// It runs over perfContextShapes because the overhead is O(workspaces): the read
// it used to pay was a full scan of the workspace table with a JSON unmarshal per
// row, so the shape with twelve workspaces and the shape with one are the two
// ends of the same line.
func BenchmarkPerf_SetChatTitle(
	b *testing.B,
) {
	args := json.RawMessage(`{"title":"Perf Fixture Title"}`)
	for _, shape := range perfContextShapes {
		b.Run(shape.name, func(b *testing.B) {
			s := newPerfStack(b)
			seedContextTree(b, s, shape.workspaces, shape.chatsPer)
			b.Logf("fixture: %d workspaces, %d chats each", shape.workspaces, shape.chatsPer)

			cold := measureCold(b, s, "set_chat_title", args)
			warm := measureWarm(b, s, "set_chat_title", args)
			cold.log(b, "cold")
			warm.log(b, "warm")
		})
	}
}

// treeLister answers like stubWorkspaces but records every List, and can be
// made to fail it. The count is the assertion for the laziness itself — the
// tree read is not observable in a tool's OUTPUT, only in whether it happened —
// and the failure is what proves the direction CanSee fails in.
type treeLister struct {
	all     []domain.Workspace
	lists   int
	listErr error
}

func (s *treeLister) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	for _, w := range s.all {
		if w.ID == id {
			return w, nil
		}
	}
	return domain.Workspace{}, errNotFoundForTest
}

func (s *treeLister) List(
	context.Context,
) ([]domain.Workspace, error) {
	s.lists++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.all, nil
}

func lazyResolverOn(
	t *testing.T,
	callerWs string,
	workspaces *treeLister,
) (*tools.Resolver, *tools.TokenMinter) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	return tools.NewResolver(
		m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: callerWs}},
		workspaces,
	), m
}

// set_chat_title acts on the caller's own runner and never looks at another
// workspace, so it must not pay for the tree. get_chat_log resolves a chat id
// belonging to SOME workspace and has to check it, so it must.
func TestToolSet_OnlyTheToolsThatNeedTheTreeReadIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool string
		args string
		want int
	}{
		{"set_chat_title needs no tree", "set_chat_title", `{"title":"Some Task"}`, 0},
		{"get_chat_log checks the chat's workspace", "get_chat_log", `{"chatId":"other"}`, 1},
		{"list_workspaces renders the tree", "list_workspaces", `{}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := &treeLister{all: tree()}
			m, err := tools.NewTokenMinter()
			require.NoError(t, err)
			chats := stubChats{c: domain.Chat{ID: "other", WorkspaceID: "ws-a1"}}
			res := tools.NewResolver(m,
				stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
				chats, ws)
			ts := tools.NewToolSet(tools.Deps{
				Resolver:  res,
				Chats:     &spyRenamer{},
				ChatReads: chats,
				ChatLogs:  &stubChatLogs{turns: chatTurns(1)},
				Lineage:   &stubLineage{},
			}, "RUN", m.Mint("RUN"))

			_, err = ts.Call(context.Background(), tc.tool, json.RawMessage(tc.args))
			require.NoError(t, err)
			require.Equal(t, tc.want, ws.lists)
		})
	}
}

type stubRunners struct {
	r   agents.Runner
	err error
}

func (s stubRunners) Get(context.Context, string) (agents.Runner, error) { return s.r, s.err }

type stubChats struct {
	c    domain.Chat
	list []domain.Chat
}

func (s stubChats) Get(context.Context, string) (domain.Chat, error) { return s.c, nil }

func (s stubChats) ListChats(context.Context) ([]domain.Chat, error) {
	return s.list, nil
}

type stubWorkspaces struct{ all []domain.Workspace }

func (s stubWorkspaces) Get(_ context.Context, id string) (domain.Workspace, error) {
	for _, w := range s.all {
		if w.ID == id {
			return w, nil
		}
	}
	return domain.Workspace{}, apperrNotFound()
}

func (s stubWorkspaces) List(context.Context) ([]domain.Workspace, error) { return s.all, nil }

// The tree used by every case below.
//
//	proj home (home)
//	  repo-default (git, IsDefault)
//	    ws-a
//	      ws-a1
//	    ws-b
//	other-repo-ws (a different repo, same project)
func tree() []domain.Workspace {
	return []domain.Workspace{
		{ID: "home", ProjectID: "P", Kind: domain.WorkspaceKindHome},
		{ID: "repo-default", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, IsDefault: true},
		{ID: "ws-a", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "repo-default"},
		{ID: "ws-a1", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "ws-a"},
		{ID: "ws-b", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "repo-default"},
		{ID: "other-repo-ws", ProjectID: "P", RepoID: "R2", Kind: domain.WorkspaceKindGit},
	}
}

func resolverOn(t *testing.T, callerWs string) (*tools.Resolver, *tools.TokenMinter) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	return tools.NewResolver(
		m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()},
	), m
}

func visibleIDs(c tools.Caller) []string {
	all, _ := c.Visible()
	out := make([]string, 0, len(all))
	for _, w := range all {
		out = append(out, w.ID)
	}
	return out
}

func apperrNotFound() error { return errNotFoundForTest }

var errNotFoundForTest = errors.New("not found")

// stubChatsByWorkspace holds the fixture keyed by owning workspace and hands
// list_workspaces the flat whole-table read the real store gives it, stamping
// each chat with the workspace its fixture key names. Keying the fixture by
// workspace is what lets a test prove chats are bucketed to the right workspace
// — and that chats of a workspace the caller cannot see are dropped rather than
// rendered.
//
// calls counts the reads: the point of A3 is that a caller seeing V workspaces
// makes ONE, so a count is the only assertion that can fail if the loop ever
// comes back.
type stubChatsByWorkspace struct {
	byWS  map[string][]domain.Chat
	calls int
}

func (s *stubChatsByWorkspace) Get(context.Context, string) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (s *stubChatsByWorkspace) ListChats(
	_ context.Context,
) ([]domain.Chat, error) {
	s.calls++
	var out []domain.Chat
	for wsID, chats := range s.byWS {
		for _, chat := range chats {
			chat.WorkspaceID = wsID
			out = append(out, chat)
		}
	}
	return out, nil
}

func listWorkspacesToolsOn(
	t *testing.T,
	callerWs string,
	byWS map[string][]domain.Chat,
) (*tools.ToolSet, *stubChatsByWorkspace) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()})
	chats := &stubChatsByWorkspace{byWS: byWS}
	return tools.NewToolSet(tools.Deps{
		Resolver: res, ChatReads: chats,
	}, "RUN", m.Mint("RUN")), chats
}

// workspaceHeaders extracts the unindented header lines render.RenderWorkspaces emits
// — one per visible workspace, "* " prefixed for the caller's own — so a test
// can assert on exactly the workspace set without a chat row (indented, and
// carrying free-typed title text) being mistaken for one. Without this, a
// substring check alone could not tell "ws-a" from "ws-a1".
func workspaceHeaders(out string) []string {
	var headers []string
	for _, line := range strings.Split(out, "\n") {
		if line == "" || strings.HasPrefix(line, " ") {
			continue
		}
		headers = append(headers, line)
	}
	return headers
}

type stubChatLogs struct {
	turns []tools.ChatTurn
	read  []string
}

func (s *stubChatLogs) ReadChatLog(
	_ context.Context,
	chatID string,
) ([]tools.ChatTurn, error) {
	s.read = append(s.read, chatID)
	return s.turns, nil
}

// chatTurns builds n turns whose bodies carry their own 1-based number, so a
// test can name the exact turn it expects at each end of a window instead of
// merely counting lines.
func chatTurns(n int) []tools.ChatTurn {
	out := make([]tools.ChatTurn, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, tools.ChatTurn{
			Speaker: "user",
			Body:    fmt.Sprintf("turn-%d", i),
		})
	}
	return out
}

// stubLineage is an tools.ChatLineageReader whose answer is fixed per chat
// id. An id with no entry has no chat ancestors, which is what nearly every chat
// in these fixtures is: unthreaded, at the panel root.
//
// It records what it was ASKED about, not merely how often. get_chat_log has to
// resolve the TARGET's lineage rather than the caller's — a caller is an
// ancestor exactly when the target's chain names it — and a stub that only
// counted calls would pass just as happily on the walk taken from the wrong end.
type stubLineage struct {
	byChat map[string][]string
	err    error
	asked  []string
}

func (s *stubLineage) Ancestors(
	_ context.Context,
	chatID string,
) ([]string, error) {
	s.asked = append(s.asked, chatID)
	if s.err != nil {
		return nil, s.err
	}
	return s.byChat[chatID], nil
}

// chatLogToolsOn builds a ToolSet on ws-a whose ChatReader resolves the named
// chat into the given workspace, and whose target chat is threaded off nobody.
func chatLogToolsOn(
	t *testing.T,
	target domain.Chat,
	logs *stubChatLogs,
) *tools.ToolSet {
	t.Helper()
	return chatLogToolsUnder(t, target, logs, &stubLineage{})
}

// chatLogToolsUnder is chatLogToolsOn with the Chats-panel tree spelled out, for
// the tests that turn on where the target sits relative to the caller.
func chatLogToolsUnder(
	t *testing.T,
	target domain.Chat,
	logs *stubChatLogs,
	lineage *stubLineage,
) *tools.ToolSet {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	chats := stubChats{c: target}
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		chats, stubWorkspaces{all: tree()})
	return tools.NewToolSet(tools.Deps{
		Resolver: res, ChatReads: chats, ChatLogs: logs, Lineage: lineage,
	}, "RUN", m.Mint("RUN"))
}

// twoHunkFile is a REALISTIC two-hunk diff of one file, which the single-hunk
// fixture every other anchor test uses cannot be: with one hunk there is no
// "between the hunks", no "spans two hunks", and no way to tell "inside a hunk"
// apart from "inside the first hunk".
//
// The numbers are a diff git could actually emit. Hunk 1 turns 7 old lines into 9
// new ones, so everything below it shifts by +2 — which is why hunk 2 starts at
// old 40 and new 42, and why the two sides of this file do not agree about which
// lines are changed.
//
//	right (new): 12..20 and 42..48
//	left  (old): 12..18 and 40..45
//
// The gap between the hunks is unchanged context, and it is wide: git's default is
// 3 context lines, so a function whose body is longer than that has its signature
// outside the hunk covering the body. That is the ordinary case, not a corner one.
func twoHunkFile() []gitdomain.FileOutline {
	return []gitdomain.FileOutline{{
		Path: "src/auth.go",
		Hunks: []gitdomain.HunkShape{
			{OldStart: 12, OldLines: 7, NewStart: 12, NewLines: 9},
			{OldStart: 40, OldLines: 6, NewStart: 42, NewLines: 7},
		},
	}}
}

func anchorArgs(start, end int, side string) string {
	return fmt.Sprintf(
		`{"filePath":"src/auth.go","startLine":%d,"endLine":%d,"side":%q,"body":"x"}`,
		start, end, side,
	)
}

// rejectionRangeList captures the whole list of ranges a rejection offered, read
// back out of the message exactly as a model would read it — up to the first "."
// or "(", which is where the list ends and the caveats or the next sentence begin.
var rejectionRangeList = regexp.MustCompile(`are: ([\d, -]+?)\s*[.(]`)

// offeredRanges is every range a rejection told the model it could re-anchor
// inside.
func offeredRanges(t *testing.T, message string) [][2]int {
	t.Helper()
	found := rejectionRangeList.FindStringSubmatch(message)
	require.Len(t, found, 2, "the rejection offered no ranges at all: %s", message)
	out := make([][2]int, 0, 2)
	for _, r := range strings.Split(found[1], ", ") {
		lo, hi, ok := strings.Cut(r, "-")
		require.True(t, ok, "%q is not a range", r)
		start, err := strconv.Atoi(lo)
		require.NoError(t, err)
		end, err := strconv.Atoi(hi)
		require.NoError(t, err)
		out = append(out, [2]int{start, end})
	}
	return out
}

// manyHunkFile is a file with more changed ranges than a rejection may print:
// thirty hunks, three lines each, ten lines apart. A real file can carry up to a
// thousand (MaxOutlineHunksPerFile), so the list has to be bounded.
func manyHunkFile() []gitdomain.FileOutline {
	hunks := make([]gitdomain.HunkShape, 0, 30)
	for i := 1; i <= 30; i++ {
		hunks = append(hunks, gitdomain.HunkShape{
			OldStart: 10 * i, OldLines: 3, NewStart: 10 * i, NewLines: 3,
		})
	}
	return []gitdomain.FileOutline{{Path: "src/auth.go", Hunks: hunks}}
}

// replyStore is the thread port for the reply-dedup tests: the read half answers
// every Get with one of its threads, the write half records every reply, and err
// makes the write fail on demand.
//
// It is separate from spyThreads rather than an extension of it because the
// failure case needs a write that fails, and widening a double every other test in
// the package already depends on is how a fixture quietly changes what those tests
// mean.
type replyStore struct {
	threads []domain.ReviewThread
	replied []reviewthread.ReplyInput
	err     error
}

func (s *replyStore) Get(_ context.Context, id string) (domain.ReviewThread, error) {
	for _, t := range s.threads {
		if t.ID == id {
			return t, nil
		}
	}
	return domain.ReviewThread{}, apperrNotFound()
}

func (s *replyStore) ListByWorkspace(_ context.Context, wsID string) ([]domain.ReviewThread, error) {
	out := make([]domain.ReviewThread, 0, len(s.threads))
	for _, t := range s.threads {
		if t.WsID == wsID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *replyStore) Open(
	_ context.Context,
	in reviewthread.OpenInput,
	_ time.Time,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{ID: "new-thread", WsID: in.WsID}, nil
}

// Reply hands back an aggregate carrying the reply's own body, so a test can tell
// the fan-out payload of a real write apart from a cached one.
func (s *replyStore) Reply(
	_ context.Context,
	in reviewthread.ReplyInput,
	now time.Time,
) (domain.ReviewThread, error) {
	if s.err != nil {
		return domain.ReviewThread{}, s.err
	}
	s.replied = append(s.replied, in)
	for _, t := range s.threads {
		if t.ID != in.ID {
			continue
		}
		t.Messages = append(t.Messages, domain.ReviewMessage{
			ID: in.MessageID, Author: in.Author, IsAgent: in.IsAgent, Body: in.Body, CreatedAt: now,
		})
		return t, nil
	}
	return domain.ReviewThread{}, apperrNotFound()
}

func (s *replyStore) Resolve(_ context.Context, id string) (domain.ReviewThread, error) {
	return domain.ReviewThread{ID: id}, nil
}

// repliedIDs is the thread each recorded reply targeted, in order.
func (s *replyStore) repliedIDs() []string {
	out := make([]string, 0, len(s.replied))
	for _, in := range s.replied {
		out = append(out, in.ID)
	}
	return out
}

// replyFixture is the reply write surface with every long-lived dependency held
// separately, so a retry can be issued through a SECOND ToolSet over the same
// store, dedup map and broadcaster — which is the production shape: a ToolSet is
// built per MCP request, so the original call and the retry that follows a broken
// pipe are served by different ToolSets over one set of daemon-lived dependencies.
type replyFixture struct {
	ts        *tools.ToolSet
	store     *replyStore
	broadcast *spyThreadBroadcast
	idem      *tools.Idempotency
}

func replyOn(t *testing.T, callerWs string, threads ...domain.ReviewThread) *replyFixture {
	t.Helper()
	return newReplyIdemFixture(
		t, callerWs,
		&replyStore{threads: threads}, tools.NewIdempotency(), &spyThreadBroadcast{},
	)
}

// retryOn is the same caller arriving again on a fresh ToolSet.
func (f *replyFixture) retryOn(t *testing.T, callerWs string) *replyFixture {
	t.Helper()
	return newReplyIdemFixture(t, callerWs, f.store, f.idem, f.broadcast)
}

// withoutDedupMap is the miswired daemon: every other dependency present, no
// Idempotency at all. reply_to_review_thread still registers (see reviewTools), so
// this is reachable rather than theoretical.
func (f *replyFixture) withoutDedupMap(t *testing.T, callerWs string) *replyFixture {
	t.Helper()
	return newReplyIdemFixture(t, callerWs, f.store, nil, f.broadcast)
}

func newReplyIdemFixture(
	t *testing.T,
	callerWs string,
	store *replyStore,
	idem *tools.Idempotency,
	broadcast *spyThreadBroadcast,
) *replyFixture {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{
			ID:            "RUN",
			CurrentChatID: "CHAT",
			WorkspaceID:   callerWs,
			ProviderID:    callerProviderID,
		}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()})
	return &replyFixture{
		ts: tools.NewToolSet(tools.Deps{
			Resolver:        res,
			Threads:         store,
			ThreadWrites:    store,
			Idempotency:     idem,
			ThreadBroadcast: broadcast.fn(),
		}, "RUN", m.Mint("RUN")),
		store:     store,
		broadcast: broadcast,
		idem:      idem,
	}
}

func (f *replyFixture) reply(args string) (string, error) {
	return f.ts.Call(context.Background(), "reply_to_review_thread", json.RawMessage(args))
}

func replyArgs(threadID, body, key string) string {
	return fmt.Sprintf(`{"threadId":%q,"body":%q,"idempotencyKey":%q}`, threadID, body, key)
}

// scopeFixture is one review served by BOTH review tools at once: the file list
// and geometry get_review_scope renders, and the outline post_review_comment
// validates against.
//
// The two come off the same stub for the same reason they come off one
// gitdomain.ReviewScope in production — GetScope loads the geometry through the
// very cache entry GetOutline serves (see branchreview's
// TestGetScope_GeometryMatchesGetOutlineAgainstRealGit). A fixture that fed them
// from two independent fields would let the surface's whole point — that a range
// this tool prints is a range that tool accepts — pass here while being false in
// production.
func scopeFixture(
	t *testing.T,
	base string,
	files []gitdomain.ReviewFileSummary,
	outline []gitdomain.FileOutline,
) *postFixture {
	t.Helper()
	f := postOn(t, "ws-a", outline)
	f.review.base = base
	f.review.files = files
	return f
}

func (f *postFixture) scope(args string) (string, error) {
	return f.ts.Call(context.Background(), "get_review_scope", json.RawMessage(args))
}

// twoHunkScope is the review the anchor tests already measure against, seen from
// the scope side: one modified file whose two hunks put the sides out of step
// (right 12-20 and 42-48, left 12-18 and 40-45), plus a pure addition, so the
// rendering is exercised on a file that has both sides and one that has only the
// right.
func twoHunkScope() ([]gitdomain.ReviewFileSummary, []gitdomain.FileOutline) {
	files := []gitdomain.ReviewFileSummary{
		{Path: "src/auth.go", Status: gitdomain.GitFileStatusModified, Additions: 11, Deletions: 5},
		{Path: "src/new.go", Status: gitdomain.GitFileStatusAdded, Additions: 40},
	}
	outline := append(twoHunkFile(), gitdomain.FileOutline{
		Path:  "src/new.go",
		Hunks: []gitdomain.HunkShape{{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 40}},
	})
	return files, outline
}

// scopeSideRanges matches one side's group on a rendered range line: the label
// and the comma-separated ranges after it. It cannot match the legend, which
// names both sides but carries no line numbers.
var scopeSideRanges = regexp.MustCompile(`(right|left) ((?:\d+-\d+)(?:, \d+-\d+)*)`)

// scopeAnchor is one anchor a model could read off get_review_scope's output and
// hand straight to post_review_comment: a file, a side and a line range.
type scopeAnchor struct {
	path  string
	side  string
	start int
	end   int
}

// anchorsFromScope reads every anchor the rendered scope offered, the way a model
// reads it: an unindented file row names the file, and the indented line under it
// carries that file's ranges per side.
//
// It parses the RENDERED TEXT rather than the fixture deliberately. The property
// under test is that what the tool PRINTS is legal — a test that fed the
// fixture's own hunks back into the validator would prove only that the fixture
// agrees with itself, which it does by construction and which no model ever sees.
func anchorsFromScope(
	t *testing.T,
	out string,
) []scopeAnchor {
	t.Helper()
	var found []scopeAnchor
	path := ""
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "    ") {
			// A file row ends in its path. The fixtures' paths carry no spaces,
			// which is what lets the last field be the path.
			if fields := strings.Fields(line); len(fields) == 4 && strings.HasPrefix(fields[1], "+") {
				path = fields[3]
			}
			continue
		}
		for _, group := range scopeSideRanges.FindAllStringSubmatch(line, -1) {
			found = append(found, parseScopeRanges(t, path, group[1], group[2])...)
		}
	}
	return found
}

func parseScopeRanges(
	t *testing.T,
	path string,
	side string,
	group string,
) []scopeAnchor {
	t.Helper()
	out := make([]scopeAnchor, 0, 2)
	for _, r := range strings.Split(group, ", ") {
		lo, hi, ok := strings.Cut(r, "-")
		require.True(t, ok, "%q is not a range", r)
		start, err := strconv.Atoi(lo)
		require.NoError(t, err)
		end, err := strconv.Atoi(hi)
		require.NoError(t, err)
		out = append(out, scopeAnchor{path: path, side: side, start: start, end: end})
	}
	return out
}

// manyHunkScopeFile is one file with more changed ranges than a row may print.
func manyHunkScopeFile(path string, hunks int) gitdomain.FileOutline {
	out := gitdomain.FileOutline{Path: path}
	for i := 1; i <= hunks; i++ {
		out.Hunks = append(out.Hunks, gitdomain.HunkShape{
			OldStart: 10 * i, OldLines: 3, NewStart: 10 * i, NewLines: 3,
		})
	}
	return out
}

// rangeList renders the fixture's own ranges the way the tool does, from the
// first to the nth, so the expectation is spelled out rather than copied.
func rangeList(from, to int) string {
	parts := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		parts = append(parts, fmt.Sprintf("%d-%d", 10*i, 10*i+2))
	}
	return strings.Join(parts, ", ")
}

// stubThreadReader is the ThreadReader test double. It records the wsID it was
// last asked for, which is how the caller-scoping tests prove a tool cannot be
// steered at another workspace, and every id Get was asked for, which is how the
// threadId tests prove a named thread is reached by ONE lookup rather than by
// scanning a workspace.
//
// ListByWorkspace FILTERS by WsID rather than handing back the whole fixture,
// the way the real store does. Without that, a thread the LISTING should never
// have shown would appear in it, and the descendant test — whose whole point is
// that threadId reaches a thread the listing cannot — would pass vacuously.
type stubThreadReader struct {
	list     []domain.ReviewThread
	lastWsID string
	gets     []string
}

func (s *stubThreadReader) ListByWorkspace(_ context.Context, wsID string) ([]domain.ReviewThread, error) {
	s.lastWsID = wsID
	out := make([]domain.ReviewThread, 0, len(s.list))
	for _, t := range s.list {
		if t.WsID == wsID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *stubThreadReader) Get(_ context.Context, id string) (domain.ReviewThread, error) {
	s.gets = append(s.gets, id)
	for _, t := range s.list {
		if t.ID == id {
			return t, nil
		}
	}
	return domain.ReviewThread{}, apperrNotFound()
}

// stubReviewReader is the ReviewReader test double, recording the wsID each
// method was last asked for and how many times the scope was resolved. outline
// is what GetOutline hands back, which is the diff geometry post_review_comment
// validates every anchor against.
type stubReviewReader struct {
	base       string
	files      []gitdomain.ReviewFileSummary
	outline    []gitdomain.FileOutline
	lastWsID   string
	scopeCalls int
}

// GetScope hands back the SAME outline GetOutline does, because the real one
// does: branchreview loads the scope's geometry through the very cache entry
// GetOutline serves, off one resolved ref. A stub with a separate geometry field
// would let get_review_scope advertise ranges post_review_comment refuses — the
// one failure the two-in-one shape exists to make impossible — and no test here
// could see it.
func (s *stubReviewReader) GetScope(_ context.Context, ws domain.Workspace) (gitdomain.ReviewScope, error) {
	s.lastWsID = ws.ID
	s.scopeCalls++
	return gitdomain.ReviewScope{Base: s.base, Files: s.files, Outline: s.outline}, nil
}

func (s *stubReviewReader) GetOutline(_ context.Context, wsID, _ string) ([]gitdomain.FileOutline, error) {
	s.lastWsID = wsID
	return s.outline, nil
}

// stubThreadWriter is the ThreadWriter test double. It records EVERY Open input,
// which is what lets the rejection tests assert the store was never written to
// rather than merely that an error came back — an implementation that validated
// after writing would return the same error and still leave the floating comment.
type stubThreadWriter struct {
	opens   []reviewthread.OpenInput
	replies []reviewthread.ReplyInput
	nextID  int
	err     error
}

func (s *stubThreadWriter) Open(
	_ context.Context,
	in reviewthread.OpenInput,
	now time.Time,
) (domain.ReviewThread, error) {
	if s.err != nil {
		return domain.ReviewThread{}, s.err
	}
	s.opens = append(s.opens, in)
	s.nextID++
	return domain.ReviewThread{
		ID:        fmt.Sprintf("thread-%d", s.nextID),
		WsID:      in.WsID,
		FilePath:  in.FilePath,
		StartLine: in.StartLine,
		EndLine:   in.EndLine,
		Side:      in.Side,
		Status:    domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{{
			ID: in.MessageID, Author: in.Author, IsAgent: in.IsAgent, Body: in.Body, CreatedAt: now,
		}},
		CreatedAt: now,
	}, nil
}

func (s *stubThreadWriter) Reply(
	_ context.Context,
	in reviewthread.ReplyInput,
	_ time.Time,
) (domain.ReviewThread, error) {
	s.replies = append(s.replies, in)
	return domain.ReviewThread{}, nil
}

func (s *stubThreadWriter) Resolve(_ context.Context, _ string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

// spyThreadBroadcast is the ThreadBroadcast test double. Fan-out is the only thing
// that puts an agent's finding in front of a user who is already looking at the
// review pane, so every test that stores a comment also checks the frame, and every
// test that rejects one checks that no frame was emitted.
type spyThreadBroadcast struct {
	frames []broadcastFrame
}

type broadcastFrame struct {
	thread    domain.ReviewThread
	projectID string
	repoID    string
}

func (s *spyThreadBroadcast) fn() tools.ThreadBroadcast {
	return func(thread domain.ReviewThread, projectID, repoID string) {
		s.frames = append(s.frames, broadcastFrame{thread: thread, projectID: projectID, repoID: repoID})
	}
}

// callerProviderID is the provider the fixture's runner is on. post_review_comment
// attributes findings to it, so the tests can prove the author came from the
// runner rather than from a constant.
const callerProviderID = "claude"

// reviewToolsetOn builds a ToolSet with the given review-surface deps on a
// caller resolved to ws-a, mirroring toolsetOn's fixture but letting each
// review test control the review deps it cares about independently.
func reviewToolsetOn(
	t *testing.T,
	threads tools.ThreadReader,
	review tools.ReviewReader,
) (*tools.ToolSet, string) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	tok := m.Mint("RUN")
	deps := tools.Deps{
		Resolver:        res,
		Threads:         threads,
		Review:          review,
		ThreadWrites:    &stubThreadWriter{},
		Idempotency:     tools.NewIdempotency(),
		ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
	}
	return tools.NewToolSet(deps, "RUN", tok), tok
}

// postFixture is the post_review_comment write surface: one caller plus every
// double it writes through, so a test can assert on the store, the fan-out and the
// outline reader together.
type postFixture struct {
	ts        *tools.ToolSet
	writer    *stubThreadWriter
	review    *stubReviewReader
	broadcast *spyThreadBroadcast
	idem      *tools.Idempotency
}

// postOn builds a fresh fixture whose caller resolves to callerWs and whose review
// contains outline.
func postOn(
	t *testing.T,
	callerWs string,
	outline []gitdomain.FileOutline,
) *postFixture {
	t.Helper()
	return newPostFixture(
		t, callerWs, outline,
		&stubThreadWriter{}, tools.NewIdempotency(), &spyThreadBroadcast{},
	)
}

// retryOn builds a SECOND fixture sharing this one's store, dedup map and
// broadcaster. That is the production shape of a retry: a ToolSet is built per MCP
// request, so the original call and its retry are served by different ToolSets over
// the same long-lived dependencies.
func (f *postFixture) retryOn(
	t *testing.T,
	callerWs string,
	outline []gitdomain.FileOutline,
) *postFixture {
	t.Helper()
	return newPostFixture(t, callerWs, outline, f.writer, f.idem, f.broadcast)
}

func newPostFixture(
	t *testing.T,
	callerWs string,
	outline []gitdomain.FileOutline,
	writer *stubThreadWriter,
	idem *tools.Idempotency,
	broadcast *spyThreadBroadcast,
) *postFixture {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{
			ID:            "RUN",
			CurrentChatID: "CHAT",
			WorkspaceID:   callerWs,
			ProviderID:    callerProviderID,
		}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()})
	review := &stubReviewReader{outline: outline}
	deps := tools.Deps{
		Resolver:        res,
		Threads:         &stubThreadReader{},
		Review:          review,
		ThreadWrites:    writer,
		Idempotency:     idem,
		ThreadBroadcast: broadcast.fn(),
	}
	return &postFixture{
		ts:        tools.NewToolSet(deps, "RUN", m.Mint("RUN")),
		writer:    writer,
		review:    review,
		broadcast: broadcast,
		idem:      idem,
	}
}

func (f *postFixture) post(args string) (string, error) {
	return f.ts.Call(context.Background(), "post_review_comment", json.RawMessage(args))
}

// outlineWithHunk is the smallest review a post can anchor into: one file, one
// hunk.
func outlineWithHunk(path string, hunk gitdomain.HunkShape) []gitdomain.FileOutline {
	return []gitdomain.FileOutline{{Path: path, Hunks: []gitdomain.HunkShape{hunk}}}
}

// authHunk is the fixture hunk every anchor test measures against: on the right it
// covers lines 40..49 inclusive.
func authHunk() []gitdomain.FileOutline {
	return outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
	})
}

// spyThreads records every write so a test can assert a rejected call never
// reached the store. thread is what Get returns.
type spyThreads struct {
	thread   domain.ReviewThread
	opened   []reviewthread.OpenInput
	replied  []reviewthread.ReplyInput
	resolved []string
}

// repliedIDs is the thread ids every recorded reply targeted, in order — what the
// scope tests assert on, which care only about WHICH thread a write reached.
func (s *spyThreads) repliedIDs() []string {
	out := make([]string, 0, len(s.replied))
	for _, in := range s.replied {
		out = append(out, in.ID)
	}
	return out
}

func (s *spyThreads) Get(context.Context, string) (domain.ReviewThread, error) {
	return s.thread, nil
}

func (s *spyThreads) ListByWorkspace(context.Context, string) ([]domain.ReviewThread, error) {
	return []domain.ReviewThread{s.thread}, nil
}

func (s *spyThreads) Open(_ context.Context, in reviewthread.OpenInput, _ time.Time) (domain.ReviewThread, error) {
	s.opened = append(s.opened, in)
	return domain.ReviewThread{ID: "new-thread", WsID: in.WsID}, nil
}

func (s *spyThreads) Reply(_ context.Context, in reviewthread.ReplyInput, _ time.Time) (domain.ReviewThread, error) {
	s.replied = append(s.replied, in)
	return s.thread, nil
}

func (s *spyThreads) Resolve(_ context.Context, id string) (domain.ReviewThread, error) {
	s.resolved = append(s.resolved, id)
	return s.thread, nil
}

// reviewToolsOn builds a ToolSet whose caller sits on ws-a (so it sees ws-a and
// ws-a1, and NOT repo-default, ws-b or other-repo-ws).
//
// It wires a broadcaster (discarded here, not asserted on) because
// canWriteReviewThread now requires ThreadBroadcast to be non-nil before
// reply_to_review_thread/resolve_review_thread are even registered — the brief's
// original fixture omitted it, which is what let a reply that failed to fan out
// still count as "done". Tests that need to assert on the fan-out itself use
// newReplyResolveFixture below instead.
func reviewToolsOn(t *testing.T, threads *spyThreads) *tools.ToolSet {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	return tools.NewToolSet(tools.Deps{
		Resolver:        res,
		Threads:         threads,
		ThreadWrites:    threads,
		ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
	}, "RUN", m.Mint("RUN"))
}

// replyResolveFixture is reviewToolsOn's fixture PLUS a wired broadcaster: the
// brief's own reviewToolsOn leaves ThreadBroadcast nil (registration must not
// depend on it — see canWriteReviewThread), so the fan-out tests need their own
// fixture that supplies one and exposes it for assertions.
type replyResolveFixture struct {
	ts        *tools.ToolSet
	threads   *spyThreads
	broadcast *spyThreadBroadcast
}

func newReplyResolveFixture(t *testing.T, callerWs string, thread domain.ReviewThread) *replyResolveFixture {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()})
	threads := &spyThreads{thread: thread}
	broadcast := &spyThreadBroadcast{}
	deps := tools.Deps{
		Resolver:        res,
		Threads:         threads,
		ThreadWrites:    threads,
		ThreadBroadcast: broadcast.fn(),
	}
	return &replyResolveFixture{
		ts:        tools.NewToolSet(deps, "RUN", m.Mint("RUN")),
		threads:   threads,
		broadcast: broadcast,
	}
}

// The preamble is a DIRECTIVE, so it must never name a capability the agent does
// not actually have. Every `x_y`-shaped token in it has to be a registered tool.
// toolNamePattern matches `x_y`-shaped tokens: lowercase words joined by
// underscores, which is the shape of every tool name on the surface (and of
// nothing else the preamble's prose uses — ordinary hyphen/underscore-free
// words like "crowbar" or "review" never match).
var toolNamePattern = regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`)

func TestCapabilitiesPreamble_OnlyNamesRegisteredTools(t *testing.T) {
	// toolsetOn, not reviewToolsOn: the registered set must be the WHOLE surface.
	// reviewToolsOn wires three tools, so the first preamble to name (say)
	// post_review_comment would fail this test claiming a registered tool "is not
	// a registered tool" — a guard that fires on the correct change and stays
	// silent on the wrong one.
	ts, _ := toolsetOn(t, &spyRenamer{})
	registered := map[string]bool{}
	for _, tool := range ts.Tools() {
		registered[tool.Name] = true
	}
	require.Len(t, registered, 8, "the preamble must be checked against the whole surface")

	preamble := config.GetPrompts().CapabilitiesInstruction
	require.NotEmpty(t, preamble)
	for _, word := range toolNamePattern.FindAllString(preamble, -1) {
		require.True(t, registered[word],
			"the preamble names %q, which is not a registered tool", word)
	}

	// The scan above now has real work to do — the preamble names set_chat_title,
	// so the loop body runs. Keep proving the matcher independently anyway: if a
	// future preamble stopped naming any tool, the scan would go quiet and this
	// assertion is what stops it passing vacuously again.
	require.Equal(t,
		[]string{"delete_review_thread"},
		toolNamePattern.FindAllString("use delete_review_thread and the crowbar review tools", -1),
		"the tool-name matcher must catch a tool-shaped token and ignore ordinary prose")
}

// attributedReviewToolsOn is reviewToolsOn with the runner's provider and current
// chat actually set. reviewToolsOn's runner deliberately carries neither, so a
// test built on it could not tell attribution read from the caller apart from
// attribution left blank.
func attributedReviewToolsOn(
	t *testing.T,
	threads *spyThreads,
	providerID string,
	chatID string,
) *tools.ToolSet {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{
			ID:            "RUN",
			CurrentChatID: chatID,
			WorkspaceID:   "ws-a",
			ProviderID:    providerID,
		}},
		stubChats{c: domain.Chat{ID: chatID, WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	return tools.NewToolSet(tools.Deps{
		Resolver:        res,
		Threads:         threads,
		ThreadWrites:    threads,
		ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
	}, "RUN", m.Mint("RUN"))
}

// reviewWriteToolNames is the set of tools that WRITE to a review thread, named
// explicitly rather than derived from a fixture.
//
// Explicitly, because the guards below are about a property of THESE tools and a
// fixture can only offer the tools it happened to wire: the schema guard used to
// iterate a fixture whose Deps left Chats, ChatReads and ChatLogs nil, so it
// walked five of the eight registered tools with nothing pinning the count — a
// guard that would have gone on passing had every write tool disappeared.
//
// It cannot simply be "all eight". get_chat_log's schema legitimately carries a
// chatId, because naming the chat to read is the whole tool; the rule being
// guarded is that no tool takes attribution FOR A WRITE, not that the string
// never appears.
var reviewWriteToolNames = []string{
	"post_review_comment",
	"reply_to_review_thread",
	"resolve_review_thread",
}

type spyRenamer struct {
	runnerID, title, source string
	calls                   int
}

func (s *spyRenamer) RenameByRunner(_ context.Context, runnerID, title, source string) error {
	s.calls++
	s.runnerID, s.title, s.source = runnerID, title, source
	return nil
}

func toolsetOn(t *testing.T, renamer tools.ChatRenamer) (*tools.ToolSet, string) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	tok := m.Mint("RUN")
	// EVERY port is wired here (with empty-returning stubs) so the shared toolset
	// fixture always advertises every registered tool group — which is what makes
	// TestToolSet_RespectsToolCeiling and TestToolSet_NoToolAcceptsAScopeArgument
	// below guard the whole surface rather than just set_chat_title. A port left
	// out here silently narrows both guards to the tools that happen to remain,
	// which is how they were vacuous before.
	deps := tools.Deps{
		Resolver:        res,
		Chats:           renamer,
		Threads:         &stubThreadReader{},
		Review:          &stubReviewReader{},
		ThreadWrites:    &stubThreadWriter{},
		Idempotency:     tools.NewIdempotency(),
		ChatReads:       stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		ChatLogs:        &stubChatLogs{},
		Lineage:         &stubLineage{},
		ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
	}
	return tools.NewToolSet(deps, "RUN", tok), tok
}

func TestToolSet_AdvertisesSetChatTitle(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	names := []string{}
	for _, tool := range ts.Tools() {
		names = append(names, tool.Name)
		require.NotEmpty(t, tool.Description, "%s has no description — it is the whole trigger budget", tool.Name)
		require.NotEmpty(t, tool.InputSchema)
	}
	require.Contains(t, names, "set_chat_title")
}

// Global constraint: codex does not defer tool schemas, so every tool costs
// context on every codex turn.
//
// Exactly 8, not "at most 8". A LessOrEqual here is a guard that cannot fail
// for the reason it exists: it passes just as happily on a fixture that wires
// five tools as on one that wires all eight — which is precisely how a
// 5-of-8 toolsetOn hid for three tasks while this test stayed green, silently
// narrowing every other guard built on the same fixture to the tools that
// happened to remain.
func TestToolSet_RespectsToolCeiling(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	require.Len(t, ts.Tools(), 8)
}

// No tool may take a scope argument — authority comes from the runner, never
// from something the model can type.
func TestToolSet_NoToolAcceptsAScopeArgument(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	forbidden := []string{"workspaceId", "workspace_id", "projectId", "project_id", "repoId", "repo_id", "runnerId", "segment"}
	for _, tool := range ts.Tools() {
		for _, f := range forbidden {
			require.NotContains(t, string(tool.InputSchema), f,
				"tool %s exposes %s; scope must never be an argument", tool.Name, f)
		}
	}
}

func TestToolSet_SetChatTitleRenamesTheCallersRunner(t *testing.T) {
	spy := &spyRenamer{}
	ts, _ := toolsetOn(t, spy)

	out, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"Refactor auth"}`))
	require.NoError(t, err)
	require.Contains(t, out, "Refactor auth")

	require.Equal(t, 1, spy.calls)
	require.Equal(t, "RUN", spy.runnerID)
	require.Equal(t, "Refactor auth", spy.title)
	// source=agent so a user-locked title is never clobbered.
	require.Equal(t, "agent", spy.source)
}

func TestToolSet_SetChatTitleRejectsEmptyTitle(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"   "}`))
	require.Error(t, err)
}

func TestToolSet_UnknownToolErrors(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	_, err := ts.Call(context.Background(), "rm_rf", json.RawMessage(`{}`))
	require.Error(t, err)
}

func TestToolSet_BadTokenCannotReachAnyTool(t *testing.T) {
	spy := &spyRenamer{}
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{}, stubWorkspaces{all: tree()})
	ts := tools.NewToolSet(tools.Deps{Resolver: res, Chats: spy}, "RUN", "forged")

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.ErrorIs(t, err, tools.ErrUnauthorized)
	require.Zero(t, spy.calls, "an unauthorized call must never reach a tool handler")
}

// toolsetGatedOn builds the full surface with a ToolAccess port under the test's
// control, recording every provider it was asked about — which is what proves
// the question is asked about the RESOLVED caller's provider rather than about
// something the relay could have supplied.
func toolsetGatedOn(
	t *testing.T,
	renamer tools.ChatRenamer,
	access func(providerID string) (bool, error),
) (*tools.ToolSet, *[]string) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{
			ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a", ProviderID: "codex",
		}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	asked := &[]string{}
	deps := tools.Deps{
		Resolver: res,
		Chats:    renamer,
		ToolAccess: func(_ context.Context, providerID string) (bool, error) {
			*asked = append(*asked, providerID)
			return access(providerID)
		},
	}
	return tools.NewToolSet(deps, "RUN", m.Mint("RUN")), asked
}

func TestToolSet_RefusesEveryCallWhenTheProvidersToolsAreOff(t *testing.T) {
	spy := &spyRenamer{}
	ts, asked := toolsetGatedOn(t, spy, func(string) (bool, error) { return false, nil })

	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))

	require.ErrorIs(t, err, tools.ErrToolsDisabled)
	require.Zero(t, spy.calls, "a refused call must never reach a tool handler")
	require.Equal(t, []string{"codex"}, *asked,
		"the switch must be read for the RESOLVED caller's provider")
}

func TestToolSet_ServesTheCallWhenTheProvidersToolsAreOn(t *testing.T) {
	spy := &spyRenamer{}
	ts, asked := toolsetGatedOn(t, spy, func(string) (bool, error) { return true, nil })

	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"Fine"}`))

	require.NoError(t, err)
	require.Equal(t, 1, spy.calls)
	require.Equal(t, []string{"codex"}, *asked)
}

// A preference the daemon cannot READ must not be guessed at in the permissive
// direction: the whole point of the switch is that the user decided, and a
// storage failure is not permission.
func TestToolSet_RefusesWhenTheSwitchCannotBeRead(t *testing.T) {
	spy := &spyRenamer{}
	ts, _ := toolsetGatedOn(t, spy, func(string) (bool, error) {
		return false, errors.New("provider preference store is down")
	})

	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))

	require.Error(t, err)
	require.Zero(t, spy.calls, "an unreadable preference must fail closed")
}

// The switch is consulted BEFORE the caller is even known to be calling a real
// tool, so a provider with its tools off gets the same answer whatever it asks
// for — including a tool that does not exist. The alternative leaks which tools
// this daemon registers to a caller that may not use any of them.
func TestToolSet_ADisabledProviderCannotProbeForToolNames(t *testing.T) {
	ts, _ := toolsetGatedOn(t, &spyRenamer{}, func(string) (bool, error) { return false, nil })

	_, err := ts.Call(context.Background(), "rm_rf", json.RawMessage(`{}`))

	require.ErrorIs(t, err, tools.ErrToolsDisabled)
}
