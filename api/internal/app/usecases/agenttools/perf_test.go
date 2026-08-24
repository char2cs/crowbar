package agenttools_test

// Baseline measurement for Phase A of
// docs/superpowers/plans/2026-07-30-agent-surface-hardening.md.
//
// What these benchmarks report is COUNTS, not just wall time. The costs Phase A
// removes — a diff ref resolved twice, a workspace aggregate folded three times,
// one full chat-table scan per visible workspace — are all duplicated work, and
// duplicated work is nearly invisible in ns/op: the second merge-base on the same
// refs hits git's page cache and the OS's, so it can cost a tenth of the first
// while still being an entire subprocess spawn. A fix that removes it must be
// provable by something that does not depend on how warm the cache was, which is
// what a spawn count is.
//
// Three independent instruments, none of which required a production change:
//
//   - the perf ring (internal/perf) already times every git subprocess under
//     "git.<subcommand>" and every repo-lock acquisition. Arming it in-process
//     and reading Snapshot back gives an exact per-subcommand spawn count.
//   - GIT_TRACE, git's own facility, pointed at a file. The perf ring buckets by
//     subcommand, so `git diff --numstat` and `git diff --name-status` share one
//     name; the trace lines carry full argv, which is what proves the repeated
//     merge-base calls are on the SAME refs rather than three different ones.
//   - counting decorators over the workspace repository, the chat read port, and
//     a GORM query callback on the chat read model. The last one is what makes
//     the O(V·C) shape of list_workspaces measurable rather than merely asserted:
//     it counts rows the SQL layer actually materialised.
//
// Run with:
//
//	go test -run='^$' -bench=Perf -benchtime=10x \
//	    ./internal/app/usecases/agenttools/...

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

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
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	"github.com/char2cs/crowbar/api/internal/perf"
)

// perfScatterEvery is how many lines apart a changed line is placed. Git's
// default context is 3 lines, so changes 10 apart never coalesce and the hunk
// count is exactly the changed-line count — which is what lets the fixture claim
// a hunk total rather than estimate one.
const perfScatterEvery = 10

// perfRunnerID names the single runner every measured call authenticates as.
const perfRunnerID = "perf-runner"

// ── counting seams ───────────────────────────────────────────────────────────

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

// countingChats adapts the chat repository to agenttools.ChatReader — the same
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

// ── harness ──────────────────────────────────────────────────────────────────

// perfStack is the production wiring of the agent tool surface over real
// repositories, a real git engine and a real repo on disk, with the counting
// seams spliced in.
type perfStack struct {
	tools       *agenttools.ToolSet
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

	minter, err := agenttools.NewTokenMinter()
	require.NoError(b, err)

	review := branchreview.New(
		workspaces,
		threads,
		repoStore,
		enginegit.New(),
		func() time.Time { return time.Unix(1_000_000, 0).UTC() },
	)

	tools := agenttools.NewToolSet(agenttools.Deps{
		Resolver:  agenttools.NewResolver(minter, runners, chats, workspaces),
		Review:    review,
		Chats:     perfRenamer{},
		ChatReads: chats,
		Metrics:   agenttools.NewMetrics(),
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

// ── git fixture ──────────────────────────────────────────────────────────────

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

// ── domain fixture ───────────────────────────────────────────────────────────

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

// ── measurement ──────────────────────────────────────────────────────────────

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
	b.Logf("%s: wall=%.1fms/call git=%.1f spawns (%.1fms) locks=%.1f "+
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
		b.Logf("    %-22s %.1f spawns/call  %6.1fms/call  %5.1fms/spawn",
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

// ── benchmarks ───────────────────────────────────────────────────────────────

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
