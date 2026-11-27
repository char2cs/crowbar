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
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	"github.com/char2cs/crowbar/api/internal/perf"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

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

func longBody(n int) string {
	return strings.Repeat("x", n)
}

func TestToolSet_RecordsEveryCallIncludingUnauthorized(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}}, stubWorkspaces{all: tree()})

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

func TestToolSet_RecordsSuccessfulCalls(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
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

func TestToolSet_FoldsUnknownToolNamesIntoOneBucket(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
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

func TestToolSet_FoldsAToolThisSetDidNotRegister(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})

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

func TestToolSet_NilMetricsStillRegistersAllNineTools(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	require.Len(t, ts.Tools(), 9)

	out, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.NoError(t, err)
	require.Contains(t, out, "x")
}

const perfScatterEvery = 10

const perfRunnerID = "perf-runner"

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

type countingReconciler struct {
	opens atomic.Int64
}

func (c *countingReconciler) OnOpen(
	_ context.Context,
	_ string,
) {
	c.opens.Add(1)
}

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

type perfRenamer struct{}

func (perfRenamer) RenameByRunner(context.Context, string, string, string) error { return nil }

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

type perfStack struct {
	tools       *tools.ToolSet
	workspaces  *countingWorkspaces
	chats       *countingChats
	chatQueries *queryCounter
	reconciler  *countingReconciler
	chatStore   agentchat.EventStore
	runners     perfRunners
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
	runners := perfRunners{}
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
		Resolver:  tools.NewResolver(minter, runners, workspaces),
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
	rm, err := workspace.OpenReadModel(adapters.WorkspaceView())
	require.NoError(b, err)
	axb := asynx.New[domain.Workspace]().
		WithEventStore(adapters.WorkspaceES()).WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	workspace.Subscriber(rm)(axb)
	ax, err := axb.Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	repo, err := workspace.New(ax, adapters.WorkspaceES(), rm, workspace.WithReconciler(reconciler))
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
	rm, err := reviewthread.OpenReadModel(adapters.ReviewThreadView())
	require.NoError(b, err)
	axb := asynx.New[domain.ReviewThread]().
		WithEventStore(es).WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	reviewthread.Subscriber(rm)(axb)
	ax, err := axb.Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	threads, err := reviewthread.New(ax, es, rm, func(domain.ReviewThread) {})
	require.NoError(b, err)
	return threads
}

func newPerfChatStore(
	b *testing.B,
	adapters *adapter.Container,
) agentchat.EventStore {
	b.Helper()
	rm, err := agentchat.OpenReadModel(adapters.AgentChatReadDB())
	require.NoError(b, err)
	axb := asynx.New[domain.Chat]().
		WithEventStore(adapters.AgentChatES()).WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	agentchat.Subscriber(rm)(axb)
	ax, err := axb.Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	store, err := agentchat.NewEventSourced(context.Background(), ax, adapters.AgentChatES(), rm, func(agentchat.ChatEvent) {}, testutil.Placements(b))
	require.NoError(b, err)
	return store
}

type perfRunners map[string]agents.Runner

func (p perfRunners) RunnerByID(runnerID string) (agents.Runner, bool) {
	r, ok := p[runnerID]
	return r, ok
}

func (perfRunners) RelayRunner(context.Context, string) (agents.Runner, bool, error) {
	return agents.Runner{}, false, nil
}

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
		Provisioning: domain.WorkspaceProvisioned,
	}, perfNow())
	require.NoError(b, err)
	seedRunnerOn(b, s, "perf-ws")
}

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
			ID:           fmt.Sprintf("perf-ws-%d", i),
			RepoID:       perfRepoID,
			ProjectID:    perfProjectID,
			Branch:       fmt.Sprintf("feature/perf-%d", i),
			Kind:         domain.WorkspaceKindGit,
			IsDefault:    i == 0,
			Provisioning: domain.WorkspacePlaceholder,
		}, perfNow())
		require.NoError(b, err)
		for j := range chatsPer {
			_, err := s.chatStore.Create(ctx, agentchat.CreateInput{
				ID:          fmt.Sprintf("perf-chat-%d-%d", i, j),
				WorkspaceID: fmt.Sprintf("perf-ws-%d", i),
				Type:        domain.ChatTypeChat,
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
		Type:        domain.ChatTypeChat,
		Now:         perfNow(),
	})
	require.NoError(b, err)
	s.runners[perfRunnerID] = agents.Runner{
		ID: perfRunnerID, WorkspaceID: wsID, ProviderID: "perf-provider",
		TerminalSession: "perf-term", CurrentChatID: chatID,
	}
}

type perfBucket struct {
	n       int
	totalMS float64
}

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

var perfReviewSpecs = []perfDiffSpec{
	{name: "40files_200hunks", files: 40, hunks: 5},
	{name: "1file_1hunk", files: 1, hunks: 1},
	{name: "200files_10000hunks", files: 200, hunks: 50},
}

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

type perfContextShape struct {
	name       string
	workspaces int
	chatsPer   int
}

var perfContextShapes = []perfContextShape{
	{name: "12ws_5chats", workspaces: 12, chatsPer: 5},
	{name: "1ws_1chat", workspaces: 1, chatsPer: 1},
}

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
		workspaces,
	), m
}

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
				stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}}, ws)
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
	r        agents.Runner
	relay    string
	relayErr error
}

func (s stubRunners) RunnerByID(string) (agents.Runner, bool) { return s.r, s.r.ID != "" }

func (s stubRunners) RelayRunner(_ context.Context, relayID string) (agents.Runner, bool, error) {
	if s.relayErr != nil {
		return agents.Runner{}, false, s.relayErr
	}
	return s.r, s.relay != "" && s.relay == relayID && s.r.ID != "", nil
}

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
		stubWorkspaces{all: tree()})
	chats := &stubChatsByWorkspace{byWS: byWS}
	return tools.NewToolSet(tools.Deps{
		Resolver: res, ChatReads: chats,
	}, "RUN", m.Mint("RUN")), chats
}

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

func chatLogToolsOn(
	t *testing.T,
	target domain.Chat,
	logs *stubChatLogs,
) *tools.ToolSet {
	t.Helper()
	return chatLogToolsUnder(t, target, logs, &stubLineage{})
}

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
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}}, stubWorkspaces{all: tree()})
	return tools.NewToolSet(tools.Deps{
		Resolver: res, ChatReads: chats, ChatLogs: logs, Lineage: lineage,
	}, "RUN", m.Mint("RUN"))
}

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

var rejectionRangeList = regexp.MustCompile(`are: ([\d, -]+?)\s*[.(]`)

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

func manyHunkFile() []gitdomain.FileOutline {
	hunks := make([]gitdomain.HunkShape, 0, 30)
	for i := 1; i <= 30; i++ {
		hunks = append(hunks, gitdomain.HunkShape{
			OldStart: 10 * i, OldLines: 3, NewStart: 10 * i, NewLines: 3,
		})
	}
	return []gitdomain.FileOutline{{Path: "src/auth.go", Hunks: hunks}}
}

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

func (s *replyStore) repliedIDs() []string {
	out := make([]string, 0, len(s.replied))
	for _, in := range s.replied {
		out = append(out, in.ID)
	}
	return out
}

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

func (f *replyFixture) retryOn(t *testing.T, callerWs string) *replyFixture {
	t.Helper()
	return newReplyIdemFixture(t, callerWs, f.store, f.idem, f.broadcast)
}

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

var scopeSideRanges = regexp.MustCompile(`(right|left) ((?:\d+-\d+)(?:, \d+-\d+)*)`)

type scopeAnchor struct {
	path  string
	side  string
	start int
	end   int
}

func anchorsFromScope(
	t *testing.T,
	out string,
) []scopeAnchor {
	t.Helper()
	var found []scopeAnchor
	path := ""
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "    ") {
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

func manyHunkScopeFile(path string, hunks int) gitdomain.FileOutline {
	out := gitdomain.FileOutline{Path: path}
	for i := 1; i <= hunks; i++ {
		out.Hunks = append(out.Hunks, gitdomain.HunkShape{
			OldStart: 10 * i, OldLines: 3, NewStart: 10 * i, NewLines: 3,
		})
	}
	return out
}

func rangeList(from, to int) string {
	parts := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		parts = append(parts, fmt.Sprintf("%d-%d", 10*i, 10*i+2))
	}
	return strings.Join(parts, ", ")
}

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

type stubReviewReader struct {
	base       string
	files      []gitdomain.ReviewFileSummary
	outline    []gitdomain.FileOutline
	lastWsID   string
	scopeCalls int
}

func (s *stubReviewReader) GetScope(_ context.Context, ws domain.Workspace) (gitdomain.ReviewScope, error) {
	s.lastWsID = ws.ID
	s.scopeCalls++
	return gitdomain.ReviewScope{Base: s.base, Files: s.files, Outline: s.outline}, nil
}

func (s *stubReviewReader) GetOutline(_ context.Context, wsID, _ string) ([]gitdomain.FileOutline, error) {
	s.lastWsID = wsID
	return s.outline, nil
}

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

const callerProviderID = "claude"

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

type postFixture struct {
	ts        *tools.ToolSet
	writer    *stubThreadWriter
	review    *stubReviewReader
	broadcast *spyThreadBroadcast
	idem      *tools.Idempotency
}

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

func outlineWithHunk(path string, hunk gitdomain.HunkShape) []gitdomain.FileOutline {
	return []gitdomain.FileOutline{{Path: path, Hunks: []gitdomain.HunkShape{hunk}}}
}

func authHunk() []gitdomain.FileOutline {
	return outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
	})
}

type spyThreads struct {
	thread   domain.ReviewThread
	opened   []reviewthread.OpenInput
	replied  []reviewthread.ReplyInput
	resolved []string
}

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

func reviewToolsOn(t *testing.T, threads *spyThreads) *tools.ToolSet {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	return tools.NewToolSet(tools.Deps{
		Resolver:        res,
		Threads:         threads,
		ThreadWrites:    threads,
		ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
	}, "RUN", m.Mint("RUN"))
}

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

var toolNamePattern = regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`)

func TestCapabilitiesPreamble_OnlyNamesRegisteredTools(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	registered := map[string]bool{}
	for _, tool := range ts.Tools() {
		registered[tool.Name] = true
	}
	require.Len(t, registered, 9, "the preamble must be checked against the whole surface")

	preamble := config.GetPrompts().CapabilitiesInstruction
	require.NotEmpty(t, preamble)
	for _, word := range toolNamePattern.FindAllString(preamble, -1) {
		require.True(t, registered[word],
			"the preamble names %q, which is not a registered tool", word)
	}

	require.Equal(t,
		[]string{"delete_review_thread"},
		toolNamePattern.FindAllString("use delete_review_thread and the crowbar review tools", -1),
		"the tool-name matcher must catch a tool-shaped token and ignore ordinary prose")
}

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
		stubWorkspaces{all: tree()})
	return tools.NewToolSet(tools.Deps{
		Resolver:        res,
		Threads:         threads,
		ThreadWrites:    threads,
		ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
	}, "RUN", m.Mint("RUN"))
}

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
		stubWorkspaces{all: tree()})
	tok := m.Mint("RUN")
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
		ThreadChanges:   reviewthread.NewChanges(),
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

func TestToolSet_RespectsToolCeiling(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	require.Len(t, ts.Tools(), 9)
}

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
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}}, stubWorkspaces{all: tree()})
	ts := tools.NewToolSet(tools.Deps{Resolver: res, Chats: spy}, "RUN", "forged")

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.ErrorIs(t, err, tools.ErrUnauthorized)
	require.Zero(t, spy.calls, "an unauthorized call must never reach a tool handler")
}

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

func TestToolSet_RefusesWhenTheSwitchCannotBeRead(t *testing.T) {
	spy := &spyRenamer{}
	ts, _ := toolsetGatedOn(t, spy, func(string) (bool, error) {
		return false, errors.New("provider preference store is down")
	})

	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))

	require.Error(t, err)
	require.Zero(t, spy.calls, "an unreadable preference must fail closed")
}

func TestToolSet_ADisabledProviderCannotProbeForToolNames(t *testing.T) {
	ts, _ := toolsetGatedOn(t, &spyRenamer{}, func(string) (bool, error) { return false, nil })

	_, err := ts.Call(context.Background(), "rm_rf", json.RawMessage(`{}`))

	require.ErrorIs(t, err, tools.ErrToolsDisabled)
}
