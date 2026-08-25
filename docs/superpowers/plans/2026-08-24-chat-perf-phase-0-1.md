# Chat Performance Phase 0 + Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the agent chat a perf harness that can fail (Phase 0), and kill the per-second re-render storm the spec measured at 225ms/tick @ 800 loaded messages (Phase 1) — without touching rendering architecture, which is Phase 2+.

**Architecture:** No new subsystems. A dev-only Go fixture seeder writes directly into the existing asynx-backed chat/activity event stores so a large synthetic chat exists to measure against. A pure TS budget-comparison module sits beside the perf instrumentation that already ships. The four Phase 1 fixes are narrow, independent edits to already-identified hot paths: one SQL query direction, one O(1) lookup replacing an O(n) scan per row, one `memo()`, one backward-walk-to-forward-pass rewrite.

**Tech Stack:** Go (backend, asynx event-sourced repositories, GORM/sqlite), React/TypeScript (frontend, Vitest).

**Spec:** `docs/superpowers/specs/2026-08-24-agent-chat-performance-plan.md` — this plan implements Phase 0 ("a harness that can fail") and Phase 1 ("stop the per-second storm") only. Phases 2-6 are separate, later plans.

## Global Constraints

- Unit tests live in `web/src/__tests__/` mirroring `web/src/`, using `@/` imports, per this repo's `CLAUDE.md`. Go tests live beside their source as `<source>_test.go`, per this repo's existing convention (e.g. `agentactivity_test.go` beside `activity.go`).
- Component files are kebab-case (`web/src/features/agent/hooks/use-scroll-frame-span.ts`, already the case for existing files this plan touches).
- Store selectors are narrow (`useXxxStore((s) => s.field)`); `useXxxStore.getState()` only inside effects/handlers; stores never import from `components/`. (No store changes are needed in this plan — noted for the implementer's awareness since these files sit next to store-consuming code.)
- **Already landed, do not redo:** commit `8d364d51` (`perf(chat): add chat.open, chat.scroll.frame, chat.stream.token spans`) already wired `markStart`/`markEnd` calls into `agent-chat-view.tsx`, a new `web/src/features/agent/hooks/use-scroll-frame-span.ts` hook, `agent-transcript.tsx`'s scroll handler, and `markdown-message.tsx`'s token-parse path, using the existing `web/src/lib/perf/instrumentation.ts`. This plan's Phase 0 tasks build on top of that instrumentation (a seeder to exercise it at scale, and a budget-comparison gate to make it fail on regression) — they do not touch the mark call sites again.
- `web/src/lib/perf/instrumentation.ts` (already shipped, read but do not modify): `markStart(name)` / `markEnd(name)` wrap `performance.mark`/`performance.measure` and are no-ops unless `perfEnabled()` (true in dev, or when `window.__CROWBAR_PERF__` is set). Every `measure` entry lands in the capped ring `window.__measures` (2000 entries) via `installPerfObserver()`. `__resetPerfForTests()` clears all state between tests.
- Live verification of anything that touches rendering or the perf marks happens in the real dev Tauri app via the Tauri MCP bridge (`mcp__tauri__*`), never headless/jsdom-only — this repo's established convention. The dev daemon's `CROWBAR_HOME` is this worktree's isolated one; never the production socket.
- This worktree's branch (`feature/chat-wrapping`) is shared with a concurrent sibling session doing unrelated backend work under `api/internal/engine/agents/`. Do not touch files under that path in this plan's tasks.

---

### Task 1: Dev-only seeded agent-chat generator (Go)

**Files:**
- Create: `api/cmd/crowbar-seed-chat/fixture.go`
- Create: `api/cmd/crowbar-seed-chat/fixture_test.go`
- Create: `api/cmd/crowbar-seed-chat/main.go`
- Modify: `Makefile:11` (add `seed-chat` to the `export CROWBAR_HOME` target list), and add a `seed-chat:` target near the existing `seed:` target (`Makefile:60-62`)

**Interfaces:**
- Consumes: `github.com/char2cs/crowbar/api/internal/adapter` (`adapter.New`, `adapter.WithHomeDir`, `(*Container).AgentChatES/SS/ReadDB`, `AgentActivityES/SS/ReadDB`, `CrowbarHome`, `Close`); `github.com/char2cs/crowbar/api/internal/app/repositories/chat` (package alias `agentchat`: `NewEventSourced`, `CreateInput`, `EventStore.Create`, `.SetTitle`); `github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity` (package alias `agentactivity`: `NewEventSourced`, `TurnInput`, `ToolInput`, `ToolResultInput`, `EventStore.AppendTurn`, `.OpenTurn`, `.CloseTurn`, `.InvokeTool`, `.CompleteTool`, `.CountTurns`, `.ToolCalls`); `github.com/char2cs/crowbar/api/internal/domain` (`Chat`, `ChatActivity`, `TurnRoleUser`, `TurnRoleAssistant`, `ToolStatusOK`); `github.com/char2cs/asynx` (`asynx.New[T]()`, `.WithEventStore`, `.WithSnapshotStore`, `.WithShardingOpts`, `.WithPanicHandler`, `.WithPublishErrorHandler`, `.Build`, `Asynx[T].WaitPublish`); `github.com/char2cs/asynx/models` (`Store`, `SnapshotStore`, `Event[T]`).
- Produces: `seedOptions{WorkspaceID, ChatID, Turns, ToolCallsPerTurn string/string/int/int}` and `func seedChat(ctx context.Context, adapters *adapter.Container, opts seedOptions) (string, error)` — Task 2's live-baseline-capture step drives this seeder via `make seed-chat` to produce a large chat to measure against.

- [ ] **Step 1: Write the failing test**

Create `api/cmd/crowbar-seed-chat/fixture_test.go`. It reads its own writes back through the same repositories `seedChat` uses, proving the seeded data is actually visible to a normal reader — not just that the write calls returned no error:

```go
package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSeedChat_WritesTheRequestedTurnsAndToolCalls(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	ctx := context.Background()
	chatID, err := seedChat(ctx, adapters, seedOptions{
		WorkspaceID:      "ws-1",
		ChatID:           "seed-test-chat",
		Turns:            3,
		ToolCallsPerTurn: 2,
	})
	require.NoError(t, err)
	require.Equal(t, "seed-test-chat", chatID)

	axAgentActivity, err := buildAsynx[domain.ChatActivity](adapters.AgentActivityES(), adapters.AgentActivitySS())
	require.NoError(t, err)
	activityStore, err := agentactivity.NewEventSourced(
		axAgentActivity, adapters.AgentActivityES(), adapters.AgentActivityReadDB(),
		adapters.CrowbarHome()+"/state/content",
	)
	require.NoError(t, err)

	turnCount, err := activityStore.CountTurns(ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, int64(6), turnCount, "3 user turns + 3 assistant turns")

	calls, err := activityStore.ToolCalls(ctx, chatID, 0, 100)
	require.NoError(t, err)
	require.Len(t, calls, 6, "3 assistant turns * 2 tool calls each")
	for _, c := range calls {
		require.Equal(t, domain.ToolStatusOK, c.Status)
	}
}

func TestSeedChat_GeneratesAChatIDWhenNoneGiven(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	chatID, err := seedChat(context.Background(), adapters, seedOptions{
		WorkspaceID: "ws-1", Turns: 1, ToolCallsPerTurn: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, chatID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./cmd/crowbar-seed-chat/... -run TestSeedChat -v`
Expected: FAIL — `seedChat`, `seedOptions`, and `buildAsynx` are undefined (package `main` has no other files yet).

- [ ] **Step 3: Write minimal implementation**

Create `api/cmd/crowbar-seed-chat/fixture.go`:

```go
// Package main is a dev-only fixture seeder: it writes a synthetic agent chat
// (alternating user/assistant turns, each assistant turn closing with N
// finished tool calls) directly into the same event-sourced stores the real
// daemon reads, bypassing the agent runner entirely. Mirrors the throwaway,
// must-not-ship convention of cmd/crowbar-seed, for a different domain (agent
// chat turns and tool calls, not git/review fixtures) — see that command's
// own header comment for the shared convention. Built with `go run -tags
// noEmbed`; never linked into the shipped binary.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// seedOptions describes one synthetic chat.
type seedOptions struct {
	WorkspaceID      string
	ChatID           string
	Turns            int
	ToolCallsPerTurn int
}

// buildAsynx mirrors internal/app.newAsynx, which is unexported and lives in
// a package this command cannot import — this command is its own main
// package, same as cmd/crowbar-seed.
func buildAsynx[T any](es asynxModels.Store, ss asynxModels.SnapshotStore) (asynx.Asynx[T], error) {
	return asynx.New[T]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		WithPanicHandler(func(ctx context.Context, evt asynxModels.Event[T], p any) {
			slog.ErrorContext(ctx, "crowbar-seed-chat: projection panic", "event", evt.EventName, "panic", p)
		}).
		WithPublishErrorHandler(func(ctx context.Context, evt asynxModels.Event[T], err error) {
			slog.ErrorContext(ctx, "crowbar-seed-chat: publish error", "event", evt.EventName, "err", err)
		}).
		Build()
}

// seedChat writes opts.Turns alternating user/assistant turns — each
// assistant turn closing with opts.ToolCallsPerTurn finished tool calls —
// into the chat and activity event stores. Returns the seeded chat's ID
// (opts.ChatID if set, else a generated one).
func seedChat(ctx context.Context, adapters *adapter.Container, opts seedOptions) (string, error) {
	chatID := opts.ChatID
	if chatID == "" {
		chatID = "seed-" + uuid.NewString()
	}

	axAgentChat, err := buildAsynx[domain.Chat](adapters.AgentChatES(), adapters.AgentChatSS())
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: asynx agent chat: %w", err)
	}
	chatStore, err := agentchat.NewEventSourced(
		axAgentChat, adapters.AgentChatES(), adapters.AgentChatReadDB(), nil,
	)
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: chat store: %w", err)
	}

	axAgentActivity, err := buildAsynx[domain.ChatActivity](adapters.AgentActivityES(), adapters.AgentActivitySS())
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: asynx agent activity: %w", err)
	}
	activityStore, err := agentactivity.NewEventSourced(
		axAgentActivity, adapters.AgentActivityES(), adapters.AgentActivityReadDB(),
		filepath.Join(adapters.CrowbarHome(), "state", "content"),
	)
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: activity store: %w", err)
	}

	now := time.Now()
	if _, err := chatStore.Create(ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: opts.WorkspaceID, Now: now,
	}); err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: create chat: %w", err)
	}
	title := fmt.Sprintf("Perf fixture: %d turns / %d tool calls", opts.Turns, opts.ToolCallsPerTurn)
	if _, err := chatStore.SetTitle(ctx, chatID, title, "user"); err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: set title: %w", err)
	}

	for i := 0; i < opts.Turns; i++ {
		now = now.Add(time.Second)
		userTurnID := fmt.Sprintf("%s-user-%d", chatID, i)
		if err := activityStore.AppendTurn(ctx, agentactivity.TurnInput{
			ChatID: chatID, TurnID: userTurnID, Role: domain.TurnRoleUser,
			Text: fmt.Sprintf("Synthetic prompt #%d", i), Now: now,
		}); err != nil {
			return "", fmt.Errorf("crowbar-seed-chat: append user turn %d: %w", i, err)
		}

		now = now.Add(time.Second)
		assistantTurnID := fmt.Sprintf("%s-assistant-%d", chatID, i)
		if err := activityStore.OpenTurn(ctx, agentactivity.TurnInput{
			ChatID: chatID, TurnID: assistantTurnID, Role: domain.TurnRoleAssistant,
			ProviderID: "claude", Now: now,
		}); err != nil {
			return "", fmt.Errorf("crowbar-seed-chat: open assistant turn %d: %w", i, err)
		}

		for j := 0; j < opts.ToolCallsPerTurn; j++ {
			now = now.Add(100 * time.Millisecond)
			toolID := fmt.Sprintf("%s-tool-%d-%d", chatID, i, j)
			if err := activityStore.InvokeTool(ctx, agentactivity.ToolInput{
				ChatID: chatID, ToolID: toolID, Name: "Read",
				Target: fmt.Sprintf("file_%d_%d.go", i, j), Now: now,
			}); err != nil {
				return "", fmt.Errorf("crowbar-seed-chat: invoke tool %d/%d: %w", i, j, err)
			}
			now = now.Add(50 * time.Millisecond)
			if err := activityStore.CompleteTool(ctx, agentactivity.ToolResultInput{
				ChatID: chatID, ToolID: toolID, Name: "Read",
				Target: fmt.Sprintf("file_%d_%d.go", i, j),
				Status: domain.ToolStatusOK, DurationMS: 50, Now: now,
			}); err != nil {
				return "", fmt.Errorf("crowbar-seed-chat: complete tool %d/%d: %w", i, j, err)
			}
		}

		now = now.Add(time.Second)
		if err := activityStore.CloseTurn(ctx, agentactivity.TurnInput{
			ChatID: chatID, TurnID: assistantTurnID, ProviderID: "claude",
			Text: fmt.Sprintf("Synthetic reply #%d, with **markdown** and `code`.", i), Now: now,
		}); err != nil {
			return "", fmt.Errorf("crowbar-seed-chat: close assistant turn %d: %w", i, err)
		}
	}

	axAgentActivity.WaitPublish()
	axAgentChat.WaitPublish()

	return chatID, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./cmd/crowbar-seed-chat/... -run TestSeedChat -v`
Expected: PASS (both `TestSeedChat_WritesTheRequestedTurnsAndToolCalls` and `TestSeedChat_GeneratesAChatIDWhenNoneGiven`).

- [ ] **Step 5: Write `main.go`**

Create `api/cmd/crowbar-seed-chat/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/char2cs/crowbar/api/internal/adapter"
)

func main() {
	workspaceID := flag.String("workspace-id", "", "existing workspace ID to attach the seeded chat to (required)")
	chatID := flag.String("chat-id", "", "chat ID to use (default: generated)")
	turns := flag.Int("turns", 20, "number of user/assistant turn pairs")
	toolCalls := flag.Int("tool-calls", 5, "finished tool calls per assistant turn")
	flag.Parse()

	if *workspaceID == "" {
		log.Fatal("crowbar-seed-chat: --workspace-id is required")
	}

	adapters, err := adapter.New()
	if err != nil {
		log.Fatalf("crowbar-seed-chat: open storage (is the dev daemon still running against this CROWBAR_HOME? stop it first): %v", err)
	}
	defer func() { _ = adapters.Close() }()

	got, err := seedChat(context.Background(), adapters, seedOptions{
		WorkspaceID:      *workspaceID,
		ChatID:           *chatID,
		Turns:            *turns,
		ToolCallsPerTurn: *toolCalls,
	})
	if err != nil {
		log.Fatalf("crowbar-seed-chat: %v", err)
	}
	fmt.Printf("seeded chat %s (%d turns, %d tool calls/assistant turn)\n", got, *turns, *toolCalls)
}
```

- [ ] **Step 6: Add the `make seed-chat` target**

In `Makefile`, change line 11 from:
```makefile
dev dev-api dev-web dev-desktop dev-bundle seed: export CROWBAR_HOME ?= $(CURDIR)/.crowbar
```
to:
```makefile
dev dev-api dev-web dev-desktop dev-bundle seed seed-chat: export CROWBAR_HOME ?= $(CURDIR)/.crowbar
```

Immediately after the existing `seed:` target (`Makefile:60-62`, currently):
```makefile
seed:
	@cd api && go run -tags noEmbed ./cmd/crowbar-seed --host $(HOST)
```
add:
```makefile
WORKSPACE_ID ?=
TURNS ?= 20
TOOL_CALLS ?= 5

seed-chat:
	@test -n "$(WORKSPACE_ID)" || (echo "usage: make seed-chat WORKSPACE_ID=<id> [TURNS=20] [TOOL_CALLS=5]" && exit 1)
	@cd api && go run -tags noEmbed ./cmd/crowbar-seed-chat --workspace-id $(WORKSPACE_ID) --turns $(TURNS) --tool-calls $(TOOL_CALLS)
```

- [ ] **Step 7: Verify it builds and vet/lint pass**

Run: `cd api && go build -tags noEmbed ./cmd/crowbar-seed-chat/... && go vet ./cmd/crowbar-seed-chat/... && golangci-lint run ./cmd/crowbar-seed-chat/...`
Expected: all three clean, no errors, no lint findings.

- [ ] **Step 8: Commit**

```bash
git add api/cmd/crowbar-seed-chat Makefile
git commit -m "feat(perf): add a dev-only agent-chat seeder for the perf harness"
```

---

### Task 2: Perf regression-budget gate

**Files:**
- Create: `web/src/lib/perf/budget.ts`
- Test: `web/src/__tests__/lib/perf/budget.test.ts`
- Create: `web/src/lib/perf/perf-baseline.json`

**Interfaces:**
- Consumes: nothing from Task 1 directly at the code level (this task's pure logic has no dependency on the seeder); the *live capture step* in this task uses `make seed-chat` (Task 1) and the Tauri MCP bridge to populate `perf-baseline.json` with real numbers.
- Produces: `export interface PerfMeasure { name: string; duration: number }`, `export interface BudgetViolation { name: string; observedMs: number; budgetMs: number; overBy: number }`, `export function summarize(measures: PerfMeasure[]): Map<string, { count: number; maxMs: number; p95Ms: number }>`, `export function checkBudgets(measures: PerfMeasure[], budgets: Record<string, number>, toleranceRatio?: number): BudgetViolation[]`. Later phases' plans read `perf-baseline.json` and update it deliberately when a phase intentionally changes a number — never edit it to silence a real regression.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/perf/budget.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { checkBudgets, summarize, type PerfMeasure } from '@/lib/perf/budget'

describe('summarize', () => {
  it('reports count, max, and p95 per distinct measure name', () => {
    const measures: PerfMeasure[] = [
      { name: 'chat.scroll.frame', duration: 8 },
      { name: 'chat.scroll.frame', duration: 9 },
      { name: 'chat.scroll.frame', duration: 40 },
      { name: 'chat.open', duration: 120 },
    ]
    const out = summarize(measures)
    expect(out.get('chat.scroll.frame')).toEqual({ count: 3, maxMs: 40, p95Ms: 40 })
    expect(out.get('chat.open')).toEqual({ count: 1, maxMs: 120, p95Ms: 120 })
  })

  it('returns an empty map for no measures', () => {
    expect(summarize([]).size).toBe(0)
  })
})

describe('checkBudgets', () => {
  const budgets = { 'chat.open': 100, 'chat.scroll.frame': 10 }

  it('reports no violations when every measure is within its budget times the tolerance', () => {
    const measures: PerfMeasure[] = [
      { name: 'chat.open', duration: 95 },
      { name: 'chat.scroll.frame', duration: 9 },
    ]
    expect(checkBudgets(measures, budgets)).toEqual([])
  })

  it('flags a measure whose p95 exceeds its budget beyond the tolerance', () => {
    const measures: PerfMeasure[] = [
      { name: 'chat.open', duration: 200 },
    ]
    const violations = checkBudgets(measures, budgets)
    expect(violations).toHaveLength(1)
    expect(violations[0]).toMatchObject({ name: 'chat.open', observedMs: 200, budgetMs: 100 })
    expect(violations[0].overBy).toBeCloseTo(100, 0)
  })

  it('ignores a measure with no budget entry', () => {
    const measures: PerfMeasure[] = [{ name: 'chat.stream.token', duration: 999 }]
    expect(checkBudgets(measures, budgets)).toEqual([])
  })

  it('allows a default 15% tolerance before flagging, and respects a custom tolerance', () => {
    const measures: PerfMeasure[] = [{ name: 'chat.open', duration: 114 }] // +14%, within default 15%
    expect(checkBudgets(measures, budgets)).toEqual([])
    expect(checkBudgets(measures, budgets, 0)).toHaveLength(1) // zero tolerance: any excess flags
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/lib/perf/budget.test.ts`
Expected: FAIL — `@/lib/perf/budget` does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/lib/perf/budget.ts`:

```ts
/**
 * Regression budgets over recorded perf spans (spec: "budgets that fail when
 * a number regresses, not as a report someone reads"). Pure functions —
 * gathering the `measures` this reads is a live-app concern, not this
 * module's.
 */
export interface PerfMeasure {
  name: string
  duration: number
}

export interface BudgetViolation {
  name: string
  observedMs: number
  budgetMs: number
  overBy: number
}

interface Summary {
  count: number
  maxMs: number
  p95Ms: number
}

function percentile(sorted: number[], p: number): number {
  if (sorted.length === 0) return 0
  const index = Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1)
  return sorted[Math.max(0, index)]
}

/** One summary row per distinct measure name: how many samples, the worst
 *  one, and the 95th percentile — a single GC-pause outlier should not by
 *  itself decide a budget verdict, but a sustained slowdown must. */
export function summarize(measures: PerfMeasure[]): Map<string, Summary> {
  const byName = new Map<string, number[]>()
  for (const m of measures) {
    const list = byName.get(m.name)
    if (list) list.push(m.duration)
    else byName.set(m.name, [m.duration])
  }
  const out = new Map<string, Summary>()
  for (const [name, durations] of byName) {
    const sorted = [...durations].sort((a, b) => a - b)
    out.set(name, {
      count: sorted.length,
      maxMs: sorted[sorted.length - 1],
      p95Ms: percentile(sorted, 95),
    })
  }
  return out
}

/** Flags every measure name whose p95 exceeds `budgets[name] * (1 +
 *  toleranceRatio)`. A name absent from `budgets` is not checked — Phase 0/1
 *  only tracks the three spans already instrumented; a later phase adds more
 *  budget entries as it instruments more spans. */
export function checkBudgets(
  measures: PerfMeasure[],
  budgets: Record<string, number>,
  toleranceRatio = 0.15,
): BudgetViolation[] {
  const summary = summarize(measures)
  const violations: BudgetViolation[] = []
  for (const [name, budgetMs] of Object.entries(budgets)) {
    const row = summary.get(name)
    if (!row) continue
    const ceiling = budgetMs * (1 + toleranceRatio)
    if (row.p95Ms > ceiling) {
      violations.push({ name, observedMs: row.p95Ms, budgetMs, overBy: row.p95Ms - budgetMs })
    }
  }
  return violations
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/lib/perf/budget.test.ts`
Expected: PASS, all 6 tests.

- [ ] **Step 5: Capture a real baseline from the live app**

This step produces real numbers — do not fabricate them. Using the dev Tauri app (per this repo's live-verification convention) and the seeder from Task 1:

1. Stop the dev daemon if running (`adapter.New` holds an exclusive lock on `CROWBAR_HOME`).
2. Run `make seed-chat WORKSPACE_ID=<a real dev workspace ID> TURNS=150 TOOL_CALLS=5` (750 tool calls, past the 500-row page the spec's §3 correctness bug concerns — this is also useful groundwork for Task 3's regression test, though Task 3 tests that at the Go layer directly).
3. Restart the dev daemon (`make dev-desktop`).
4. Open the seeded chat in the running app, using the Tauri MCP bridge (`mcp__tauri__*` tools): navigate to it, scroll the transcript a few times, let a moment pass for `chat.open` to close.
5. Read `window.__measures` via `mcp__tauri__webview_execute_js` (e.g. `JSON.stringify(window.__measures ?? [])`).
6. From that captured array, compute the p95 (via this task's own `summarize`, or by hand) for `chat.open`, `chat.scroll.frame`, and `chat.stream.token`, and write them into `web/src/lib/perf/perf-baseline.json`:

```json
{
  "chat.open": 0,
  "chat.scroll.frame": 0,
  "chat.stream.token": 0
}
```

Replace the three `0` placeholders with the captured p95 values (milliseconds, rounded to the nearest integer). These numbers ARE the deliverable of this step — do not leave them at `0`; `0` would make `checkBudgets` flag every future measurement as a regression.

- [ ] **Step 6: Add a smoke test that the baseline file is well-formed**

Add to `web/src/__tests__/lib/perf/budget.test.ts`:

```ts
import baseline from '@/lib/perf/perf-baseline.json'

describe('perf-baseline.json', () => {
  it('has a positive budget for every span this phase instruments', () => {
    for (const span of ['chat.open', 'chat.scroll.frame', 'chat.stream.token']) {
      expect(baseline).toHaveProperty(span)
      expect((baseline as Record<string, number>)[span]).toBeGreaterThan(0)
    }
  })
})
```

- [ ] **Step 7: Run the full test file to verify it passes**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/lib/perf/budget.test.ts`
Expected: PASS, all 7 tests — including the new baseline smoke test, which only passes once Step 5's real numbers are in place.

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/perf/budget.ts web/src/lib/perf/perf-baseline.json web/src/__tests__/lib/perf/budget.test.ts
git commit -m "feat(perf): add a regression-budget gate over the chat perf spans"
```

---

### Task 3: Activity cursor — default to the newest tool calls, not the oldest

**Files:**
- Modify: `api/internal/app/repositories/chat/activity/internal/store/internal/storage/queries.go` (add `ToolCallsBefore`, mirroring `TurnsBefore` at `:40-58`)
- Modify: `api/internal/app/repositories/chat/activity/internal/store/store.go` (add a thin `ToolCallsBefore` wrapper beside `ToolCalls`)
- Modify: `api/internal/app/repositories/chat/activity/activity.go` (add `ToolCallsBefore` to the `EventStore` interface beside `ToolCalls` at `:109`, and to the `eventSourced` impl beside `:338-342`)
- Modify: `api/internal/app/usecases/chat/internal/turn/observation.go` (branch in `ReadActivity`, `:245-259`)
- Test: `api/internal/app/repositories/chat/activity/agentactivity_test.go`

**Interfaces:**
- Consumes: nothing from Tasks 1-2.
- Produces: `EventStore.ToolCallsBefore(ctx context.Context, chatID string, before int64, limit int) ([]domain.ActivityToolCall, error)` — no other task in this plan consumes it, but it is the same shape as `TurnsBefore`, already consumed elsewhere in this codebase, so any later phase reading "the newest page of tool calls" reuses this rather than reinventing it.

**Context:** `api/internal/app/repositories/chat/activity/internal/store/internal/storage/queries.go`'s current `ToolCalls` (verbatim):
```go
func (s *Store) ToolCalls(
	ctx context.Context,
	chatID string,
	after int64,
	limit int,
) ([]domain.ActivityToolCall, error) {
	q := s.db.WithContext(ctx).Model(&ToolCallRow{}).Where("chat_id = ?", chatID)
	if after > 0 {
		q = q.Where("seq > ?", after)
	}
	q = q.Order("seq ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []ToolCallRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentactivity storage: tool calls: %w", err)
	}
	out := make([]domain.ActivityToolCall, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.domain())
	}
	return out, nil
}
```
Called with `after=0, limit=500` from `observation.go`'s `ReadActivity` (the default, cursor-less request every chat-open makes) — `seq > 0 ORDER BY seq ASC LIMIT 500` returns the **oldest** 500 rows. Past 500 tool calls, the rows under the most recent replies silently stop appearing. `TurnsBefore` in the same file already solves the equivalent problem for turns — `ToolCallsBefore` mirrors it exactly.

- [ ] **Step 1: Write the failing test**

In `api/internal/app/repositories/chat/activity/agentactivity_test.go`, add (matching the file's existing `newFixture(t)`/`f.repo`/`f.wait()` pattern, e.g. as used by `TestRecentToolCalls_SpansChatsNewestFirst`):

```go
func TestToolCallsBefore_ReturnsTheNewestPageInAscendingOrder(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 5; i++ {
		require.NoError(t, f.repo.InvokeTool(f.ctx, activity.ToolInput{
			ChatID: chat, ToolID: fmt.Sprintf("tool-%d", i), Name: "Read",
			Target: fmt.Sprintf("f%d.go", i), Now: t0.Add(time.Duration(i) * time.Second),
		}))
	}
	f.wait()

	got, err := f.repo.ToolCallsBefore(f.ctx, chat, 0, 3)
	require.NoError(t, err)
	require.Len(t, got, 3, "newest 3 of 5")
	assert.Equal(t, "f2.go", got[0].Target, "still ascending by seq")
	assert.Equal(t, "f3.go", got[1].Target)
	assert.Equal(t, "f4.go", got[2].Target, "newest last")
}

func TestRegression_ToolCallsBeforeDefaultsToNewestNotOldest(t *testing.T) {
	f := newFixture(t)
	const total = 520
	for i := 0; i < total; i++ {
		require.NoError(t, f.repo.InvokeTool(f.ctx, activity.ToolInput{
			ChatID: chat, ToolID: fmt.Sprintf("tool-%d", i), Name: "Read",
			Target: fmt.Sprintf("f%d.go", i), Now: t0.Add(time.Duration(i) * time.Second),
		}))
	}
	f.wait()

	got, err := f.repo.ToolCallsBefore(f.ctx, chat, 0, 500)
	require.NoError(t, err)
	require.Len(t, got, 500)
	assert.Equal(t, "f19.go", got[0].Target, "oldest of the newest-500 window")
	assert.Equal(t, "f519.go", got[499].Target, "the actual newest call is present")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/repositories/chat/activity/... -run 'TestToolCallsBefore|TestRegression_ToolCallsBefore' -v`
Expected: FAIL — `ToolCallsBefore` is undefined on `activity.EventStore`.

- [ ] **Step 3: Add `ToolCallsBefore` to the storage layer**

In `api/internal/app/repositories/chat/activity/internal/store/internal/storage/queries.go`, immediately after the existing `ToolCalls` function, add:

```go
func (s *Store) ToolCallsBefore(
	ctx context.Context,
	chatID string,
	before int64,
	limit int,
) ([]domain.ActivityToolCall, error) {
	q := s.db.WithContext(ctx).Model(&ToolCallRow{}).Where("chat_id = ?", chatID)
	if before > 0 {
		q = q.Where("seq < ?", before)
	}
	var rows []ToolCallRow
	if err := q.Order("seq DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentactivity storage: tool calls before: %w", err)
	}
	out := make([]domain.ActivityToolCall, len(rows))
	for i, r := range rows {
		out[len(rows)-1-i] = r.domain()
	}
	return out, nil
}
```

- [ ] **Step 4: Wrap it in the store layer**

In `api/internal/app/repositories/chat/activity/internal/store/store.go`, immediately after the existing `ToolCalls` wrapper (`s.heal(ctx); return s.storage.ToolCalls(...)`), add:

```go
func (s *Store) ToolCallsBefore(
	ctx context.Context,
	chatID string,
	before int64,
	limit int,
) ([]domain.ActivityToolCall, error) {
	s.heal(ctx)
	return s.storage.ToolCallsBefore(ctx, chatID, before, limit)
}
```

- [ ] **Step 5: Add it to the `EventStore` interface and `eventSourced` impl**

In `api/internal/app/repositories/chat/activity/activity.go`, add to the `EventStore` interface, immediately after the existing `ToolCalls` line (`:109`):
```go
	ToolCallsBefore(ctx context.Context, chatID string, before int64, limit int) ([]domain.ActivityToolCall, error)
```
And immediately after the existing `eventSourced.ToolCalls` method (`:338-342`):
```go
func (r *eventSourced) ToolCallsBefore(
	ctx context.Context, chatID string, before int64, limit int,
) ([]domain.ActivityToolCall, error) {
	return r.store.ToolCallsBefore(ctx, chatID, before, limit)
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd api && go test ./internal/app/repositories/chat/activity/... -run 'TestToolCallsBefore|TestRegression_ToolCallsBefore' -v`
Expected: PASS, both tests.

- [ ] **Step 7: Wire the default (cursor-less) request to use it**

In `api/internal/app/usecases/chat/internal/turn/observation.go`, replace the current unconditional call (`:247-250`):
```go
	calls, err := t.activity.ToolCalls(ctx, chatID, after, limit)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: tool calls: %w", err)
	}
```
with:
```go
	var calls []domain.ActivityToolCall
	if after > 0 {
		calls, err = t.activity.ToolCalls(ctx, chatID, after, limit)
	} else {
		calls, err = t.activity.ToolCallsBefore(ctx, chatID, 0, limit)
	}
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: tool calls: %w", err)
	}
```
Check this file's existing imports for `domain` (the package is already imported elsewhere in this file for other `ChatActivity`-adjacent types — add the import if it is not already present under its existing alias).

- [ ] **Step 8: Run the full package test suites to verify nothing else broke**

Run: `cd api && go test ./internal/app/repositories/chat/activity/... ./internal/app/usecases/chat/... -v 2>&1 | tail -60`
Expected: PASS. (`tests/integration/agent/agent_transcript_test.go` is gated behind `//go:build integration` and `requireCLI(t, "claude")` — it does not run under a plain `go test ./...` and needs a live `claude` CLI, so it is not part of this task's gate; per [[project_agent_integration_needs_live_apis]], its failures can be environment/API-outage noise unrelated to this change. It DOES call `ReadActivity(ctx, chatID, 0, 0)` — the exact default cursor-less path this task changes — so if it happens to be run later (`go test -tags integration ./tests/integration/agent/...` with a real `claude` CLI available), it is a relevant signal, just not a blocking one here.)

- [ ] **Step 9: Vet and lint**

Run: `cd api && go vet ./internal/app/repositories/chat/activity/... ./internal/app/usecases/chat/... && golangci-lint run ./internal/app/repositories/chat/activity/... ./internal/app/usecases/chat/internal/turn/...`
Expected: clean.

- [ ] **Step 10: Commit**

```bash
git add api/internal/app/repositories/chat/activity api/internal/app/usecases/chat/internal/turn/observation.go
git commit -m "fix(chat): default activity's tool-call page to the newest, not the oldest"
```

---

### Task 4: `AgentTurnTools` — group tool calls by turn once, not per row

**Files:**
- Modify: `web/src/features/agent/transcript/turn-tools.tsx` (current, full file, lines 1-31)
- Modify: `web/src/features/agent/transcript/agent-transcript.tsx` (add the derivation; change the call site at what is currently lines 119-121)
- Test: `web/src/__tests__/features/agent/transcript/turn-tools.test.tsx` — **this file already exists** (5 tests against the current `{ activity, turnId }` prop shape) and must be rewritten, not appended to; Step 1 below gives its full replacement content, converting every existing test to the new `{ callsByTurn, turnId }` shape while preserving exactly what each one proves. `web/src/__tests__/features/agent/transcript/agent-transcript.test.tsx` does not exist yet — Task 6 creates it; this task does not touch it.

**Interfaces:**
- Consumes: `AgentToolCall`, `AgentActivity` from `@/features/agent/api/agent-api` (unchanged shapes).
- Produces: `groupToolCallsByTurn(toolCalls: AgentToolCall[]): Map<string, AgentToolCall[]>`, exported from `turn-tools.tsx`. `AgentTurnTools`'s prop shape changes from `{ activity: AgentActivity; turnId: string }` to `{ callsByTurn: Map<string, AgentToolCall[]>; turnId: string }` — this is a breaking change to `AgentTurnTools`'s public props, and `agent-transcript.tsx` is its only call site (confirmed: grep found no other importer).

**Context:** `web/src/features/agent/transcript/turn-tools.tsx`, current full file:
```tsx
import type { AgentActivity } from '@/features/agent/api/agent-api'
import { describeTool, formatDuration } from '@/features/agent/lib/agent-activity'

const LIMIT = 6

export function AgentTurnTools({ activity, turnId }: { activity: AgentActivity; turnId: string }) {
  if (!turnId) return null
  const calls = activity.toolCalls
    .filter((call) => call.turnId === turnId && call.status !== 'running')
    .sort((a, b) => a.seq - b.seq)
  if (calls.length === 0) return null
  // ...renders calls.slice(0, LIMIT)
}
```
Every render does a full filter+sort over the entire `activity.toolCalls` array — the spec measured 52ms at 500 rows × 5,000 calls. `agent-transcript.tsx`'s only call site (currently lines 119-121): `{message.role === 'assistant' && (<AgentTurnTools activity={props.activity} turnId={message.turnId ?? ''} />)}`, inside the `messages.map()` loop.

- [ ] **Step 1: Write the failing test**

Replace the full contents of `web/src/__tests__/features/agent/transcript/turn-tools.test.tsx` (its current 5 tests all construct an `activity` object and pass `activity={...}` — every one is rewritten below to build a `callsByTurn` map via `groupToolCallsByTurn` instead, preserving exactly what each one proves):

```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AgentToolCall } from '@/features/agent/api/agent-api'
import { AgentTurnTools, groupToolCallsByTurn } from '@/features/agent/transcript/turn-tools'

function tool(overrides: Partial<AgentToolCall> = {}): AgentToolCall {
  return {
    id: 't1',
    turnId: 'turn-1',
    seq: 1,
    name: 'Bash',
    status: 'ok',
    hasRequest: false,
    hasResult: false,
    startedAt: '2026-08-17T12:00:00Z',
    ...overrides,
  }
}

describe('groupToolCallsByTurn', () => {
  it('groups finished calls by turn, sorted by seq, excluding running calls', () => {
    const calls = [
      tool({ id: 'a', turnId: 't1', seq: 2 }),
      tool({ id: 'b', turnId: 't1', seq: 1 }),
      tool({ id: 'c', turnId: 't2', seq: 1, status: 'running' }),
      tool({ id: 'd', turnId: 't2', seq: 2 }),
    ]
    const grouped = groupToolCallsByTurn(calls)
    expect(grouped.get('t1')?.map((c) => c.id)).toEqual(['b', 'a'])
    expect(grouped.get('t2')?.map((c) => c.id)).toEqual(['d'])
  })

  it('returns an empty map for no calls', () => {
    expect(groupToolCallsByTurn([]).size).toBe(0)
  })
})

describe('AgentTurnTools', () => {
  it('shows the finished work a reply is built on', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        callsByTurn={groupToolCallsByTurn([tool({ name: 'Grep', target: 'x.ts', durationMs: 1200 })])}
      />,
    )
    expect(screen.getByText('Grep · x.ts')).toBeInTheDocument()
    expect(screen.getByText('1.2s')).toBeInTheDocument()
  })

  it('shows only the tools of ITS turn', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        callsByTurn={groupToolCallsByTurn([
          tool({ id: 'a', name: 'Mine' }),
          tool({ id: 'b', turnId: 'turn-2', name: 'Theirs' }),
        ])}
      />,
    )
    expect(screen.getByText('Mine')).toBeInTheDocument()
    expect(screen.queryByText('Theirs')).not.toBeInTheDocument()
  })

  it('renders nothing for a reply with no tools, and omits still-running ones', () => {
    const { container } = render(
      <AgentTurnTools turnId="turn-1" callsByTurn={groupToolCallsByTurn([tool({ status: 'running' })])} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  // A reply built on a failed tool must not read as clean.
  it('marks a failed tool', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        callsByTurn={groupToolCallsByTurn([tool({ status: 'error', name: 'Bash' })])}
      />,
    )
    expect(screen.getByText('Bash').closest('li')).toHaveAttribute('data-status', 'error')
  })

  it('renders nothing without a turn id — a streaming bubble has no turn yet', () => {
    const { container } = render(
      <AgentTurnTools turnId="" callsByTurn={groupToolCallsByTurn([tool()])} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when the turn has no entry in the map', () => {
    const { container } = render(<AgentTurnTools callsByTurn={new Map()} turnId="t1" />)
    expect(container).toBeEmptyDOMElement()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/features/agent/transcript/turn-tools.test.tsx`
Expected: FAIL — `groupToolCallsByTurn` is not exported, and `AgentTurnTools` does not accept `callsByTurn`.

- [ ] **Step 3: Rewrite `turn-tools.tsx`**

Replace the full contents of `web/src/features/agent/transcript/turn-tools.tsx` with:

```tsx
import type { AgentToolCall } from '@/features/agent/api/agent-api'
import { describeTool, formatDuration } from '@/features/agent/lib/agent-activity'

/** Tool rows shown under a reply before the rest collapse into a count. */
const LIMIT = 6

/** Finished tool calls, grouped by turn and sorted by seq within each turn.
 *  Computed once per activity change (see agent-transcript.tsx), not once
 *  per row — the O(n) filter+sort this used to do inside every row's render
 *  was quadratic in conversation length. */
export function groupToolCallsByTurn(toolCalls: AgentToolCall[]): Map<string, AgentToolCall[]> {
  const byTurn = new Map<string, AgentToolCall[]>()
  for (const call of toolCalls) {
    if (call.status === 'running') continue
    const list = byTurn.get(call.turnId)
    if (list) list.push(call)
    else byTurn.set(call.turnId, [call])
  }
  for (const list of byTurn.values()) {
    list.sort((a, b) => a.seq - b.seq)
  }
  return byTurn
}

/**
 * What the agent DID to produce a reply, under the reply.
 *
 * Finished calls only — anything still running belongs to the working line, not
 * to a turn that has already been answered.
 */
export function AgentTurnTools({
  callsByTurn,
  turnId,
}: {
  callsByTurn: Map<string, AgentToolCall[]>
  turnId: string
}) {
  if (!turnId) return null
  const calls = callsByTurn.get(turnId) ?? []
  if (calls.length === 0) return null

  return (
    <ul className="tools" data-testid="agent-turn-tools">
      {calls.slice(0, LIMIT).map((call) => (
        <li key={call.id} data-status={call.status}>
          <span>{describeTool(call)}</span>
          {call.durationMs !== undefined && <span>{formatDuration(call.durationMs)}</span>}
        </li>
      ))}
      {calls.length > LIMIT && <li>+{calls.length - LIMIT} more</li>}
    </ul>
  )
}
```

- [ ] **Step 4: Update the call site in `agent-transcript.tsx`**

Add `useMemo` to the existing `react` import at the top of `web/src/features/agent/transcript/agent-transcript.tsx` (currently has no React import at all — add `import { useMemo } from 'react'` as the first import line), change the `turn-tools` import from:
```tsx
import { AgentTurnTools } from '@/features/agent/transcript/turn-tools'
```
to:
```tsx
import { AgentTurnTools, groupToolCallsByTurn } from '@/features/agent/transcript/turn-tools'
```
Inside `AgentTranscript`, immediately after the existing `const scrollFrame = useScrollFrameSpan()` line, add:
```tsx
  const callsByTurn = useMemo(() => groupToolCallsByTurn(props.activity.toolCalls), [props.activity.toolCalls])
```
And change the call site (currently):
```tsx
                {message.role === 'assistant' && (
                  <AgentTurnTools activity={props.activity} turnId={message.turnId ?? ''} />
                )}
```
to:
```tsx
                {message.role === 'assistant' && (
                  <AgentTurnTools callsByTurn={callsByTurn} turnId={message.turnId ?? ''} />
                )}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/features/agent/transcript/turn-tools.test.tsx src/__tests__/features/agent/chat/agent-chat-view.test.tsx`
Expected: PASS. `agent-chat-view.test.tsx` (there is no `agent-transcript.test.tsx` yet — Task 6 creates it) exercises `AgentTranscript` end-to-end and must still pass unchanged, proving the new call site is wired correctly.

- [ ] **Step 6: Typecheck and lint**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun tsc --noEmit && PATH="$HOME/.bun/bin:$PATH" bun run lint` (use this repo's actual lint script name from `package.json` if different from `lint`)
Expected: clean. (Note: `bunx tsc` is a different package from `bun tsc` in this repo — always use `bun tsc`.)

- [ ] **Step 7: Commit**

```bash
git add web/src/features/agent/transcript/turn-tools.tsx web/src/features/agent/transcript/agent-transcript.tsx web/src/__tests__/features/agent/transcript/turn-tools.test.tsx
git commit -m "perf(chat): group tool calls by turn once per activity change, not per row"
```

---

### Task 5: Memoize `MessageRow`; skip `applyMessages`'s rebuild on an empty page

**Files:**
- Modify: `web/src/features/agent/transcript/message-row.tsx` (wrap `MessageRow` in `memo`)
- Modify: `web/src/features/agent/hooks/use-chat-messages.ts` (guard `applyMessages`)
- Test: `web/src/__tests__/features/agent/transcript/message-row.test.tsx` — **already exists** (80 lines, one `describe('MessageRow', ...)` block covering role rendering; uses a local `row(role, text)` helper that constructs a fresh message object per call). Add a new, separate `describe` block to it (Step 1 below) — do not touch its existing tests. `web/src/__tests__/features/agent/hooks/use-chat-messages.test.ts` does not exist yet (confirmed: no test anywhere currently covers `useChatMessages`) — create it fresh.

**Interfaces:**
- Consumes: nothing from Tasks 1-4.
- Produces: `MessageRow` remains the same exported name with the same prop shape (`{ message, showProvider, providers }`) — only its identity changes (now a `memo`-wrapped component), so no caller needs to change. `applyMessages`'s exposed behavior (what `useChatMessages` returns, and when `onApply` fires) is unchanged for a non-empty page; for an empty page it now skips the array rebuild and `setMessages` call, but **must still call `onApply`** — `onApply` is what drives the prompt-queue reconciliation walk in `loadInitial`, and skipping it on an empty page would break that walk's termination check (`pendingEvidence()`/`recovery.hasMore`).

**Context:** `web/src/features/agent/transcript/message-row.tsx`'s current `MessageRow` (lines 125-133 for the signature):
```tsx
export function MessageRow({
  message,
  showProvider,
  providers,
}: {
  message: AgentChatMessage
  showProvider: boolean
  providers: AgentProvider[]
}) {
  // ...
}
```
This codebase's existing `memo()` precedent (`agent-chat-row.tsx`, `agent-chat-folder-row.tsx`): rename the function to `XComponent`, export the memoized wrapper under the original name, no custom comparator.

`web/src/features/agent/hooks/use-chat-messages.ts`'s current `applyMessages` (lines 77-92):
```ts
  const applyMessages = useCallback(
    (incoming: AgentChatMessage[]) => {
      const next = mergeMessages(messagesRef.current, incoming)
      messagesRef.current = next
      setMessages(next)
      if (next.length > 0) {
        cursorRef.current = Math.max(cursorRef.current, next.at(-1)?.sequence ?? 0)
        oldestCursorRef.current =
          oldestCursorRef.current === 0
            ? (next[0]?.sequence ?? 0)
            : Math.min(oldestCursorRef.current, next[0]?.sequence ?? oldestCursorRef.current)
      }
      onApply(next)
    },
    [onApply],
  )
```
`mergeMessages` unconditionally rebuilds a `Map` over every current message and returns a new array — even when `incoming` is `[]`, which is the common case on an unchanged poll tick (`MESSAGE_POLL_MS = 1_000`, `use-chat-messages.ts:5`). That new array reference forces `AgentTranscript`'s `messages.map()` to re-run on every tick regardless of whether anything changed.

- [ ] **Step 1: Write the failing test for `MessageRow`**

Add the following `describe` block to the END of the existing `web/src/__tests__/features/agent/transcript/message-row.test.tsx` (do not touch its existing `describe('MessageRow', ...)` block or its imports — this new block reuses the file's existing `providers` constant (line 10) and its already-imported `render`, `describe`, `expect`, `it`, `AgentChatMessage`, `MessageRow`; it needs no new imports):

```tsx
describe('MessageRow memoization', () => {
  it('does not re-render when called twice with identical prop values', () => {
    const message: AgentChatMessage = {
      turnId: 't1', sequence: 1, role: 'assistant', providerId: 'claude', text: 'hi', at: '2026-08-24T00:00:00Z',
    }
    const { rerender, container } = render(
      <MessageRow message={message} showProvider={false} providers={providers} />,
    )
    const firstHTML = container.innerHTML
    // Same object references — memo's default shallow comparator must skip this render.
    rerender(<MessageRow message={message} showProvider={false} providers={providers} />)
    expect(container.innerHTML).toBe(firstHTML)
  })
})
```

(This test proves memoization exists structurally — a `memo`-wrapped component with unchanged props does not re-run its body, which a raw DOM-content comparison alone cannot distinguish from a re-render that happens to produce identical output. Do NOT add a `vi.mock` for `markdown-message` here or anywhere in this file — the file's existing tests render the real `MarkdownMessage` and assert on its output (`.closest('strong')`, `.closest('code')`); a file-scoped mock would silently break them. If this repo's testing conventions have a preferred render-count-spy pattern already in use elsewhere for other `memo()`-wrapped components in this codebase, check `agent-chat-row.test.tsx` first and mirror that pattern instead of the above if it differs.)

- [ ] **Step 2: Run test to verify it fails or passes for the wrong reason**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/features/agent/transcript/message-row.test.tsx`
Since `MessageRow` is currently a plain function component with no internal state, this specific test may pass even before memoization (a plain function called twice with the same props also produces identical output). This is expected and does not mean Step 3 is unnecessary — proceed to Step 3 regardless; Step 2 exists to confirm the harness runs, not to gate the change on a red result here.

- [ ] **Step 3: Wrap `MessageRow` in `memo`**

In `web/src/features/agent/transcript/message-row.tsx`, add `memo` to the existing `react`... note this file currently has no top-level React import (uses no hooks) — add `import { memo } from 'react'` as the first import line. Rename the function signature at lines 125-133 from `export function MessageRow({...})` to `function MessageRowComponent({...})` (same body, same closing brace — only the `export function MessageRow` → `function MessageRowComponent` line changes), and immediately after the function's closing brace, add:
```tsx
export const MessageRow = memo(MessageRowComponent)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/features/agent/transcript/message-row.test.tsx src/__tests__/features/agent/transcript/turn-tools.test.tsx src/__tests__/features/agent/chat/agent-chat-view.test.tsx`
Expected: PASS. (`turn-tools.test.tsx` and `agent-chat-view.test.tsx` are unrelated to this change but exercise `MessageRow` transitively — run alongside to confirm the `memo` wrap did not break anything they depend on. There is no `agent-transcript.test.tsx` yet — Task 6 creates it.)

- [ ] **Step 5: Write the failing test for `applyMessages`**

Create `web/src/__tests__/features/agent/hooks/use-chat-messages.test.ts` (confirmed: no existing test file covers this hook yet). Check `listChatMessages`'s mocking convention used by `agent-chat-view.test.tsx` (which already mocks `@/features/agent/api/agent-api`'s `listMessagesFn` via `vi.hoisted`) and mirror it:

```ts
import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { useChatMessages } from '@/features/agent/hooks/use-chat-messages'

const { listChatMessagesFn } = vi.hoisted(() => ({ listChatMessagesFn: vi.fn() }))
vi.mock('@/features/agent/api/agent-api', () => ({ listChatMessages: listChatMessagesFn }))

function message(sequence: number): AgentChatMessage {
  return { turnId: `t${sequence}`, sequence, role: 'user', providerId: '', text: 'hi', at: '2026-08-24T00:00:00Z' }
}

describe('applyMessages empty-page guard', () => {
  beforeEach(() => {
    listChatMessagesFn.mockReset()
  })

  it('still calls onApply on an empty page, so queue reconciliation keeps working', async () => {
    const onApply = vi.fn()
    listChatMessagesFn.mockResolvedValue({ cursor: 0, oldestCursor: 0, hasMore: false, items: [] })
    const { result } = renderHook(() =>
      useChatMessages({
        wsId: 'ws', chatId: 'c1', providerId: 'claude', visible: true, working: false,
        turnRevision: 0, awaiting: false, onApply, pendingEvidence: () => false,
        pendingBaselines: () => [], onRecoveryExhausted: () => {},
      }),
    )
    await waitFor(() => expect(onApply).toHaveBeenCalled())
    expect(onApply).toHaveBeenCalledWith([])
  })

  it('does not create a new messages array reference across two empty pages', async () => {
    const seen: AgentChatMessage[][] = []
    listChatMessagesFn
      .mockResolvedValueOnce({ cursor: 5, oldestCursor: 1, hasMore: false, items: [message(1), message(5)] })
      .mockResolvedValue({ cursor: 5, oldestCursor: 1, hasMore: false, items: [] })
    const { result, rerender } = renderHook(() =>
      useChatMessages({
        wsId: 'ws', chatId: 'c1', providerId: 'claude', visible: true, working: true,
        turnRevision: 0, awaiting: false, onApply: (m) => seen.push(m), pendingEvidence: () => false,
        pendingBaselines: () => [], onRecoveryExhausted: () => {},
      }),
    )
    await waitFor(() => expect(result.current.messages).toHaveLength(2))
    const firstRef = result.current.messages
    await result.current.refresh()
    rerender()
    expect(result.current.messages).toBe(firstRef)
  })
})
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/features/agent/hooks/use-chat-messages.test.ts`
Expected: FAIL on the second test (`does not create a new messages array reference across two empty pages`) — `mergeMessages` currently always returns a new array. The first test should already pass (existing behavior).

- [ ] **Step 7: Guard `applyMessages`**

In `web/src/features/agent/hooks/use-chat-messages.ts`, replace the current `applyMessages` (lines 77-92):
```ts
  const applyMessages = useCallback(
    (incoming: AgentChatMessage[]) => {
      const next = mergeMessages(messagesRef.current, incoming)
      messagesRef.current = next
      setMessages(next)
      if (next.length > 0) {
        cursorRef.current = Math.max(cursorRef.current, next.at(-1)?.sequence ?? 0)
        oldestCursorRef.current =
          oldestCursorRef.current === 0
            ? (next[0]?.sequence ?? 0)
            : Math.min(oldestCursorRef.current, next[0]?.sequence ?? oldestCursorRef.current)
      }
      onApply(next)
    },
    [onApply],
  )
```
with:
```ts
  const applyMessages = useCallback(
    (incoming: AgentChatMessage[]) => {
      if (incoming.length === 0) {
        // Nothing new: skip the Map rebuild and setMessages entirely — a
        // fresh array reference here forces AgentTranscript's messages.map()
        // to re-run on every unchanged poll tick. Cursor bookkeeping has
        // nothing to advance either. onApply still fires: it drives the
        // prompt-queue recovery walk in loadInitial, which reads
        // pendingEvidence()/recovery.hasMore after every applied page,
        // empty or not.
        onApply(messagesRef.current)
        return
      }
      const next = mergeMessages(messagesRef.current, incoming)
      messagesRef.current = next
      setMessages(next)
      cursorRef.current = Math.max(cursorRef.current, next.at(-1)?.sequence ?? 0)
      oldestCursorRef.current =
        oldestCursorRef.current === 0
          ? (next[0]?.sequence ?? 0)
          : Math.min(oldestCursorRef.current, next[0]?.sequence ?? oldestCursorRef.current)
      onApply(next)
    },
    [onApply],
  )
```
(The `if (next.length > 0)` guard around cursor bookkeeping in the original is now redundant — the early return above already handles the empty case, so the non-empty branch can update cursors unconditionally.)

- [ ] **Step 8: Run test to verify it passes**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/features/agent/hooks/use-chat-messages.test.ts`
Expected: PASS, both tests.

- [ ] **Step 9: Run the full affected suite**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/features/agent/hooks src/__tests__/features/agent/transcript src/__tests__/features/agent/chat`
Expected: all PASS — this exercises `useChatMessages`'s prompt-queue recovery paths end-to-end (`loadInitial`'s baseline-forward and oldest-page-backward loops both call `applyMessages` repeatedly, including with empty recovery pages) and must still terminate correctly.

- [ ] **Step 10: Typecheck and lint**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun tsc --noEmit && PATH="$HOME/.bun/bin:$PATH" bun run lint`
Expected: clean.

- [ ] **Step 11: Commit**

```bash
git add web/src/features/agent/transcript/message-row.tsx web/src/features/agent/hooks/use-chat-messages.ts web/src/__tests__/features/agent/transcript/message-row.test.tsx web/src/__tests__/features/agent/hooks/use-chat-messages.test.ts
git commit -m "perf(chat): memoize MessageRow and skip applyMessages's rebuild on an empty page"
```

---

### Task 6: `previousAssistantProvider` — one forward pass instead of a backward walk called twice per row

**Files:**
- Modify: `web/src/features/agent/transcript/agent-transcript.tsx` (lines 41-47, and the call sites currently at lines 110-118)
- Create: `web/src/__tests__/features/agent/transcript/agent-transcript.test.tsx` — does not exist yet (confirmed); Step 1 creates it

**Interfaces:**
- Consumes: `AgentChatMessage` from `@/features/agent/api/agent-api` (unchanged).
- Produces: a module-level `providerLabelSequences(messages: AgentChatMessage[]): Set<number>` inside `agent-transcript.tsx` (not exported — used only within this file, same visibility as the `previousAssistantProvider` function it replaces).

**Context:** Current `agent-transcript.tsx` (lines 41-47):
```tsx
function previousAssistantProvider(messages: AgentChatMessage[], index: number): string {
  for (let i = index - 1; i >= 0; i--) {
    if (messages[i]?.role === 'assistant') return messages[i]?.providerId ?? ''
  }
  return ''
}
```
Called twice per assistant row at the current call site (lines 113-117, inside the `messages.map()` loop from Task 4's `callsByTurn` addition onward the surrounding structure is unchanged except for that addition):
```tsx
                <MessageRow
                  message={message}
                  providers={props.providers}
                  showProvider={
                    message.role === 'assistant' &&
                    (previousAssistantProvider(messages, index) === '' ||
                      previousAssistantProvider(messages, index) !== message.providerId)
                  }
                />
```
Semantics to preserve exactly: `showProvider` is `true` for an assistant message iff there is no earlier assistant message in the currently-loaded `messages` window, or the nearest earlier assistant message has a different `providerId`. Non-assistant rows neither reset nor count toward "nearest earlier assistant." `messages` is already the paginated window this component receives as a prop, so a forward pass over the same array inherits the exact same "resets at the window edge" behavior with no special-casing.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/transcript/agent-transcript.test.tsx` — confirmed not to exist yet, so this is `AgentTranscript`'s first dedicated unit test file (its behavior has so far only been exercised indirectly via `agent-chat-view.test.tsx`, which stays untouched by this task). `AgentTranscriptProps`'s required fields (everything without a `?`) are `messages`, `queue`, `providers`, `activity`, `working`, `loading`, `error`, `hasOlder`, `onLoadOlder`, `onRetryLoad`, `onOpenTerminal`, `onEditPrompt`, `onCancelPrompt`, `onRetryPrompt`:

```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'

describe('AgentTranscript provider labels', () => {
  it('shows the provider label on the first assistant message and on a provider change, not on consecutive same-provider replies', () => {
    const messages: AgentChatMessage[] = [
      { turnId: 't1', sequence: 1, role: 'user', providerId: '', text: 'hi', at: '' },
      { turnId: 't2', sequence: 2, role: 'assistant', providerId: 'claude', text: 'a', at: '' },
      { turnId: 't3', sequence: 3, role: 'assistant', providerId: 'claude', text: 'b', at: '' },
      { turnId: 't4', sequence: 4, role: 'assistant', providerId: 'codex', text: 'c', at: '' },
    ]
    render(
      <AgentTranscript
        messages={messages}
        queue={[]}
        providers={[]}
        activity={{ toolCalls: [], subagents: [], interruptions: [], choices: [] }}
        working={false}
        loading={false}
        error={null}
        hasOlder={false}
        onLoadOlder={() => {}}
        onRetryLoad={() => {}}
        onOpenTerminal={() => {}}
        onEditPrompt={() => {}}
        onCancelPrompt={() => {}}
        onRetryPrompt={() => {}}
      />,
    )
    // Sequence 2: first assistant message -> label shown. Sequence 3: same
    // provider as 2 -> no label. Sequence 4: provider changed -> label shown.
    const rows = screen.getAllByTestId(/^agent-message-\d+$/)
    expect(rows[1].querySelector('.meta')).not.toBeNull()
    expect(rows[2].querySelector('.meta')).toBeNull()
    expect(rows[3].querySelector('.meta')).not.toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it currently passes (baseline)**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/features/agent/transcript/agent-transcript.test.tsx`
Expected: PASS — this test documents current behavior before the rewrite; it is not meant to fail first (there is no new capability here, only a performance-motivated rewrite that must not change output). Its job is to catch a regression in Step 4.

- [ ] **Step 3: Replace the backward walk with a forward pass**

In `web/src/features/agent/transcript/agent-transcript.tsx`, replace lines 41-47:
```tsx
/** The provider of the nearest earlier assistant message, for the label. */
function previousAssistantProvider(messages: AgentChatMessage[], index: number): string {
  for (let i = index - 1; i >= 0; i--) {
    if (messages[i]?.role === 'assistant') return messages[i]?.providerId ?? ''
  }
  return ''
}
```
with:
```tsx
/** Sequences of assistant messages that should carry the provider label — the
 *  first assistant message in the loaded window, and any whose provider
 *  differs from the nearest earlier assistant message. One forward pass,
 *  computed once per messages change, replacing a backward walk that used to
 *  run twice per assistant row. */
function providerLabelSequences(messages: AgentChatMessage[]): Set<number> {
  const sequences = new Set<number>()
  let previousProvider = ''
  for (const message of messages) {
    if (message.role !== 'assistant') continue
    if (previousProvider === '' || previousProvider !== message.providerId) {
      sequences.add(message.sequence)
    }
    previousProvider = message.providerId ?? ''
  }
  return sequences
}
```
Add `useMemo` to the `react` import at the top of the file (Task 4's Step 4 already added `import { useMemo } from 'react'` if that task ran first in this plan's order — if this line is already present, do not duplicate it). Inside `AgentTranscript`, immediately after `const callsByTurn = useMemo(...)` (added by Task 4), add:
```tsx
  const providerLabelSeqs = useMemo(() => providerLabelSequences(messages), [messages])
```
And change the call site from:
```tsx
                <MessageRow
                  message={message}
                  providers={props.providers}
                  showProvider={
                    message.role === 'assistant' &&
                    (previousAssistantProvider(messages, index) === '' ||
                      previousAssistantProvider(messages, index) !== message.providerId)
                  }
                />
```
to:
```tsx
                <MessageRow
                  message={message}
                  providers={props.providers}
                  showProvider={message.role === 'assistant' && providerLabelSeqs.has(message.sequence)}
                />
```

- [ ] **Step 4: Run test to verify it still passes**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun run vitest run src/__tests__/features/agent/transcript/agent-transcript.test.tsx src/__tests__/features/agent/chat/agent-chat-view.test.tsx`
Expected: PASS, identical outcome to Step 2 — this rewrite must be behavior-preserving.

- [ ] **Step 5: Typecheck and lint**

Run: `cd web && PATH="$HOME/.bun/bin:$PATH" bun tsc --noEmit && PATH="$HOME/.bun/bin:$PATH" bun run lint`
Expected: clean. (`previousAssistantProvider` and the `index` parameter of the `messages.map()` callback may now be unused if nothing else in the file references `index` — check the lint output; if `index` is otherwise unused, remove it from the `.map((message, index) => ...)` signature, but only if this typecheck/lint step actually flags it, since some ESLint configs allow unused destructured/positional params.)

- [ ] **Step 6: Commit**

```bash
git add web/src/features/agent/transcript/agent-transcript.tsx web/src/__tests__/features/agent/transcript/agent-transcript.test.tsx
git commit -m "perf(chat): replace the per-row backward provider walk with one forward pass"
```
