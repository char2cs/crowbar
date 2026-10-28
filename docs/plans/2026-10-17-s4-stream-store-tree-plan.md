# S4: One stream, one entity store, Tree. Implementation Plan (index)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks are sequential within S4. **S4 cannot start before S3 T8 (the asynx v1 bump) has landed** (precondition P1). Which S1, S2 and S3 tasks each S4 task needs is in "Dependencies" below (refresh 2026-10-28); where it is narrower than "all of S1–S3" it is an open ruling (OR1).

**Goal:** Sidebar placement has one owner, a per-project Tree aggregate. The daemon publishes everything the client shows over one WebSocket (`/v0/stream`) with per-key versions. The client keeps one entity store fed only by that stream, and the sidebar is one row-derivation function over it. `app-sync-engine`, the IndexedDB entity cache, the sidebar's data stores, placement fields on chats and workspaces, the `folders` table, the Node aggregate, `hub`, `realtime`, `v0/ws` and `snapshots.go` are deleted.

**Architecture:** Three strands, each finished before the next begins. (1) Tree: `domain/tree` holds the node model and every placement rule as pure functions; an asynx aggregate per project (id = project id) stores it; two migration steps convert today's Node rows, folders and chat placement into it. (2) One stream: `domain/stream` declares the frames; `app/stream` is the single publisher (one lock, one version counter per daemon epoch, one bounded queue per connection) with a source per scope kind (`projects`, `chat:<id>`, `ws:<id>`, `git:<id>`, `files:<id>`, `lsp:<id>`, and `doc:<path>` for S5's host documents, OR2); every old stream endpoint moves onto it in the task that deletes it. (3) Client: `web/src/core/stream` (one socket, refcounted scopes) and `web/src/core/entities` (one versioned store); the sidebar moves to `web/src/features/workspace/tree` and becomes `deriveRows(input)` over the Tree entity and the entity store.

**Tech Stack:** Go (`api/`, `-tags noEmbed`), asynx v1 (S3 T8), gin, gorilla/websocket, gorm + SQLite, testify, the `api/tests` black-box harness, the migration engine (`api/internal/core/migrate`). React 19, zustand, TanStack Router, `@/components/ui/*`, phosphor icons, vitest + Testing Library (`web/src/__tests__/`), `idb`, Tauri MCP for the live check.

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` §3 (Tree, entity store, "the client keeps no second copy", "panes hold ids", "a chat's workspace is one field"), §4 "The one stream", §8 (delete `hub`, `realtime` lifecycles, `v0/ws`, `snapshots`, `app-sync-engine`, the IndexedDB entity cache, the sidebar stores; merge the sidebar into `features/workspace/tree`), §11 row S4. Earlier plans for what exists when S4 starts: `2026-10-17-s1-chat-ledger-plan.md` (R8 feed), `2026-10-17-s2-chatstore-plan.md` (ChatStore, R15–R17, R21), `2026-10-17-s3-operations-errors-plan.md` (operations, R7, R8, R16, R20, R21, A7), `2026-10-17-migration-engine-plan.md` (catalog, pre/post-open, ledger, R10). Executors read the sections a task cites.

## Plan files

| File | Tasks |
|---|---|
| `docs/plans/2026-10-17-s4a-tree-plan.md` | T1 pin the hierarchy (web, black box, live "before") · T2 the Tree aggregate replaces Node, `folders` and chat placement · T3 the Tree decides: explicit containers, rules, atomic moves · T4 the chat tree usecase goes |
| `docs/plans/2026-10-17-s4b-one-stream-plan.md` | T5 the one stream, with operations · T6 workspace scopes (`ws:`, `git:`, `files:`, `lsp:`); `realtime` goes · T7 the chat ledger rides the stream |
| `docs/plans/2026-10-17-s4c-entity-store-cutover-plan.md` | T8 one row-derivation function · T9 one tree UI store · T10 the `projects` scope fills the entity store; `app-sync-engine`, the IndexedDB entity cache, `hub`, `ws` and `snapshots.go` go |
| `docs/plans/2026-10-17-s4d-moves-creates-gate-plan.md` | T11 moves go through the Tree · T12 creates and removals are one Tree write · T13 gate and live check |

13 tasks, one commit each.

## Preconditions

- **P1.** S3 T8 is committed: asynx is a release after v1.0.0 that fixes NF1 (a `Get` racing `Forget` must not resurrect an aggregate), NF2 (a busy aggregate must not stall its shard) and NF7 (recovery linear per aggregate). The Tree is one aggregate per project: every placement write of a project goes through it, and `Get` is its only read, so all three findings sit on S4's hot path. If `cd api && go list -m github.com/char2cs/asynx` reports v1.0.0 or earlier, stop and report. At refresh (2026-10-28) `go.mod` still pins v0.8.1; asynx v1.1.0 was tagged 2026-10-19 and S3 T8 pins it (S3 ledger), but S3 T8 has not run.
- **P2.** The S1–S3 tasks a task depends on ("Dependencies") are committed. Before each task, `grep` every symbol its **Files** block names. When an earlier stage moved one, follow the responsibility, not the path; when it already deleted one, say so in the commit body and skip that step. Paths marked *(S2 Tn)* or *(S3 Tn)* below do not exist at refresh; that task creates them.
- **P3.** `df -h "$(git rev-parse --show-toplevel)"` shows at least 8 GB free before any build, `bun install` or `make dev-desktop`. Under 8 GB, stop and report.

## Dependencies (refresh 2026-10-28)

State at refresh: S1 T1–T12 and the gate docs merged (T9 subagent/plan entities, T10's per-chat feed WebSocket `/v0/chats/:chatId/feed`, T11's HTTP-only hook ingress, T12's step 6 `chat-ledger`); S1 T13's live part and the final review are open (the S1 LOC gate is not met, ledger). S2 has not started. S3 has T10, T11a, T11b (T9 absorbed); T1–T8 and T12 have not run. S5 T1–T7 and S6 T1–T6c merged (S5 added `/v0/documents/ws` and the realtime host watch; IndexedDB is at version 11).

| S4 task | Needs first | Why |
|---|---|---|
| T1 | S1 final review | pins today's wire; only the sidebar row and its test change |
| T2 | S3 T8 (P1) | the Tree is an asynx aggregate on v1 |
| T3 | S3 T1 (`domain/problem`, `web/src/lib/errors/`) | R19's codes are `problem` codes |
| T4 | T3 | — |
| T5 | S3 T3 (operations service, `/v0/operations`, `lib/store/operations.ts`, `run-operation.ts`) | T5 moves the operations stream onto `/v0/stream` |
| T6 | T5; S2 T4 (R16's refcounted lifecycle view holds `ws:`) | — |
| T7 | S2 T1 (append-only `FeedDelta`) and S2 T3 (ChatStore `feeds`, `wsManager.subscribe`'s `url`) | T7 re-points what S2 builds |
| T8, T9 | S3 T5 (`rows-from-operations.ts`, `create-draft.ts`) | they fold S3's row deriver and draft into the tree |
| T10 | S2 T4 and S2 T11 (ChatStore `chats/byWorkspace/scopes/telemetry`; lifecycle carries entity frames only); S3 T3 (`entity-presence.ts`) | T10 deletes the lifecycle socket S2 consolidates |
| T11–T13 | T10 | — |

**S2 ↔ S4.** S2 through T11 lands before S4 T6 (strictly: T6 needs S2 T4, T7 needs S2 T1 and T3, T10 needs S2 T11; S2 T12–T13 are chat-surface hygiene and its gate, which S4 does not touch). Shared files: `web/src/lib/ws/manager.ts` (S2 T3 adds `subscribe`'s `url`, S4 T7 deletes it), `web/src/features/agent/stores/chat-store.ts` (S2 T3/T4 create it, S4 T7/T10 shrink it to `feeds` + `outbox`), `web/src/lib/ws/chat-frame-kinds.ts` (S2 T11 trims, S4 T10 deletes), `api/internal/app/usecases/chat/feed.go` (S2 T1 changes `FeedDelta`, S4 T7 drops the feed's epoch), `api/internal/api/v0/dto/agent.go` and `endpoints/chat/handlers/` (S2 T11 deletes read DTOs and handlers, S4 T3/T11 placement), `api/internal/api/v0/container.go` `agentChatDef` (S2 T11, S4 T6/T10). S4 T1–T5 share only the last three items' Go files with S2 (`dto/agent.go`, `endpoints/chat/handlers/`, `api/v0/container.go`); they can run beside S2 if OR1 is accepted, rebased by the controller's min-merge.

---

## Global Constraints

Every task's requirements include these. Quoted from the spec, `CLAUDE.md` and the user's standing rules.

- "No feature flags, no parallel old/new code paths, no compatibility shims between stages. Each stage deletes what it replaces in the same change." Within S4, a task that moves data onto the stream deletes that data's old endpoint, its server publisher and its client subscriber in the same commit. Between T5 and T10 the daemon serves two transports, but each kind of data is on exactly one of them at every commit (R5). The legacy placement translation T3 adds is deleted by T11 (R11).
- "Same features as today, fewer lines." Nothing a user can do today is lost. Visible changes are listed under "Decisions that change what the user sees".
- No provider vocabulary in Go: no Claude or Codex names, event names or flags.
- "A fix starts with a failing test. Never fix a symptom with a timer, poll, retry, extra flag, or resync effect. If the fix needs a second copy of some state, stop and write a plan in `docs/plans/`." Every backend bug found while implementing gets a black-box `TestRegression_*`.
- The sidebar hierarchy is a hard invariant: project → repo → locked branches → workspaces → forked child workspaces; chat ↔ workspace parentage visible; one sidebar; project-home folders and chats flat at the project's top level, interleaved with repo home rows, with no synthetic "Home" container; no ghost rows; project-home rows are never forkable. T1 pins it; every later task keeps that test green unchanged except for its fixture loader.
- Go: required dependencies are constructor arguments, never nil-checked setters; no `...ForTest` methods or test-only package vars in non-test files; `fmt.Errorf("op: ctx: %w", err)`; early returns; funlen 100/50, gocyclo 15, nestif 2; one test file per source file (CI rejects `*_coverage_test.go`, `*_extra_test.go`, `*_gaps_test.go`); no `time.Sleep` in tests (channels, `require.Eventually`, the harness's stream reader).
- Web: kebab-case files; narrow selectors (`useX((s) => s.field)`), never a bare `useX()`; `getState()` only in handlers and effect bodies; stores never import `components/` or any `*.tsx`; tests in `web/src/__tests__/` mirroring `src`, `@/` imports, extend the unit's test file, ≤3 `vi.mock` per file, no Tailwind-class assertions, no reading `src/` text, no sleeps; phosphor, base-ui, dnd-kit, shiki only; Plate is the only markdown; preserve every animation ("a snap is a defect").
- Files: every production file a task creates or edits is ≤500 lines when the task commits; an edited file over 500 lines is split by responsibility in that task (each task's **Files** block names the oversize files it edits, with their `file-size-baseline.json` size at refresh). Comments explain *why* in ≤3 lines; a task that edits a file brings its comments to that rule.
- Lint suppressions name the rule and give a reason on the same line. Never loosen a lint config, coverage floor or budget. Baselines (`api/deadcode-baseline.txt`, `web/knip.json` `ignoreIssues`, `*_BASELINE` in `web/eslint.config.js`, golangci `new-from-rev`, `scripts/ratchet/*-baseline.json`) only shrink: fix an entry, delete it in the same commit; never add one.
- On-disk changes go through the migration engine's catalog (`api/internal/adapter/migrations/catalog.go`) with `Plan`/`Apply`/`Verify`, idempotent under re-run, checkpointed with `Checkpoints.Mark`, `RewritesState: true` (the engine backs up). Post-open steps sit at the catalog's Known layout (`From == To`).
- Commits stage only the files the task touched, by path (`git add <path>…`, `git rm <path>…`). Never `git add -A`. Never bare `git stash`. Never push. No PRs.
- Running tests: targeted only, never a full suite. Web: `cd web && bunx vitest run <paths>`; types `cd web && bun tsc --noEmit` (never `bunx tsc`). Before each web commit: `bunx eslint <touched> --max-warnings 0`, `bunx prettier --check <touched>`, `bun run knip`, `bun run lint:baselines`, `bun run lint:doctor`. Go, always in this form (below, "`GOTEST <pkgs>`"):

```bash
cd api && T=$(mktemp -d) && HOME=$T/home CROWBAR_HOME= go test -tags noEmbed <pkgs>; rm -rf $T
```

  (the handoff's form; `<scratchpad>/tools/env.sh` sets `PATH` when the toolchain was rebuilt). Black box ("`GOBOX <regex>`"): the same prefix with `go test -tags 'integration noEmbed' -count=1 -run '<regex>' ./tests/` (the top-level `./tests/` package only; no `-race` except on the targeted tests). Never run `-tags integration` tests in `api/tests/integration/...` or `api/tests/kit` (the kit touches provider homes); compile them only: `go vet -tags 'integration noEmbed' ./tests/...`. Never `go test ./...`, `make pr-checks`, `make test-integration`, `make test-coverage` or `make meter`. Then, from `api/`: `golangci-lint run --build-tags noEmbed ./...` (new code), `scripts/deadcode-check.sh`, `go vet -tags noEmbed ./...`.
- Safety: never connect to `~/.crowbar/crowbar.sock` or open anything under `~/.crowbar/state` (not even `ls`). Live runs use `make dev-desktop CROWBAR_HOME=<worktree>/.crowbar/s4-pin/home` (inside the git-ignored `/.crowbar/`), one dev instance at a time, on the user's machine or in the isolated noVNC container the S1/S5/S6 live gates used; never in a session that can reach the user's provider homes. Never read or write `~/.claude`, `~/.codex` or `~/.claude.json`, and run no real provider CLI (the S2 R25 fake only). No scratch files tracked in the worktree.
- IndexedDB is shared across builds by bundle id: a production build and an S4 build may open the same `crowbar` database. S4 never changes that database's version (11 at refresh: S5 replaced `editor-state` with `editor-drafts`), never deletes or creates an object store, never writes or removes `localStorage['crowbar:cache-version']` (R14).

## Rulings (open decisions resolved for S4)

| # | Decision | Ruling |
|---|---|---|
| R1 | `make ratchet` per task | As S2 R1 and S3 R1: tasks check the 500-line cap with `wc -l` on what they touched, run `node scripts/ratchet/file-size.mjs` and `node scripts/ratchet/counters.mjs`, and state the net line change per module key in the commit body. `loc-budgets` is red at base (S1 ledger R7, awaiting the user): report the delta, never raise a budget. A task that creates a module prefix or moves code between keys edits `scripts/ratchet/modules.json` and both budgets in `loc-budgets-baseline.json` by the moved amount in the same commit; `<lang>.total` is never raised. Write-backs of lower numbers happen at merge (controller `chore(ratchet)` commits) and once more in T13 (`node scripts/ratchet/<file>.mjs --update`). |
| R2 | Tree granularity | One asynx aggregate per project, aggregate id = project id, state `tree.Tree{ProjectID, Nodes map[id]Node}`. Every command decides against the whole project tree, so a move and the renumbering of both sibling levels are one event. `ShouldSnapshot` is true for every command, so `Get` reads one blob. No read model: `Get` is the read. |
| R3 | What is a node | Kinds `folder`, `repo`, `workspace`, `chat`. A repo node stands for the repo and its home checkout. A workspace node stands for every other workspace (locked branch, branch, fork), whether or not it has an owning chat. A chat node is a chat that owns no workspace (a plain chat or a thread). The project home workspace has no node: its level is the project root (`ParentID == ""`). A folder's name is on its node (`Name`); the `folders` table goes. |
| R4 | Explicit containers (T3) | After T3 every `ParentID` names a node of the same tree or `""` (project root). A repo home's chats and the repo's workspaces hang under the repo node; a workspace's chats under its workspace node; a fork under its fork parent's node unless filed in a folder. Nothing reads git lineage (`Workspace.ParentID`) to place a row. The rules (`domain/tree`, T3): no cycle; a folder never under a chat; a repo only at project level (root or a home folder); a node keeps its context (home, or the subtree of one repo); a chat directly under a chat or workspace node is bound to that node's resolved workspace (nearest workspace node, else repo node's home checkout, else the project home), while a chat filed in a folder keeps its bound workspace whatever container holds the folder (ledger F1 amendment and D6: today's cross-workspace folder filing stays legal; only chat-under-chat across workspaces is refused); a workspace's resolved parent workspace equals its fork parent (the repo's home checkout when it has none); a move whose subtree holds a working chat is refused. Facts the rules need from other aggregates (bound workspace, fork parent, home checkouts, working) are passed in the command (`tree.Facts`), never stored on nodes. |
| R5 | Two transports during T5–T10 | The old `hub`/`ws` machinery and `app/stream` coexist from T5 to T10. Each kind of data is published on exactly one of them at every commit; the task that moves a kind deletes its old publisher, endpoint and client subscriber. |
| R6 | Frame envelope | `domain/stream.Frame`, one JSON object per WebSocket message, discriminated by `t`: `ready{epoch}` (first frame of every connection) · `entity{scope, kind, id, v, data \| gone}` · `synced{scope, seq?}` (the scope's snapshot is complete; `seq` for a chat scope) · `row{scope, row}` (a ledger row, S1) · `delta{scope, delta}` (S2 R3, ephemeral) · `signal{scope, kind, data}` (git status, file change, diagnostics; not versioned) · `closed{scope, reason: resync \| deleted \| refused, problem?}`. Client → server: `{op: "sub", scope, since?}` and `{op: "unsub", scope}`. Seven frame kinds replace 9 stream definitions (`projectsDef`, `reposDef`, `agentChatDef`, `gitDef`, `filesDef`, `lspDef`, `threadsDef`, `terminalsDef`, and S5's `documentsDef`) + S1's feed + S3's operations stream. |
| R7 | Entity versions | `v` is the stream's publish sequence (uint64, starts at 1 per daemon epoch), stamped under the hub's lock. A scope's snapshot is computed and enqueued under the same lock, every entity stamped with the current sequence, before the scope is registered for live frames; a live frame therefore always carries a higher `v` than the snapshot it follows. The client applies an entity frame only when `v` is newer than what it holds; equal `v` keeps object identity. A new epoch in `ready` resets every version the client holds. Ledger scopes keep S1's `seq` and are not locked (they attach live, then read since `seq`, and dedupe by `seq`). |
| R8 | Resume | On reconnect the client re-sends `sub` for every held scope: entity scopes re-snapshot (an entity absent from the new snapshot is removed when `synced` arrives); a chat scope sends `since` = its feed's `seq`. A connection whose queue (1024 frames) overflows is closed with code 4000; the client reconnects through `wsManager`'s existing backoff and resumes. No timer is added anywhere. |
| R9 | Scope → resources | The hub keeps one refcount per scope. `ws:<id>` starts the forge poll for that workspace; `git:<id>` the file watcher and the protected-branch origin check; `files:<id>` the file watcher; `lsp:<id>` the LSP host; S5's host-document watch needs a scope too (OR2). One registry replaces the refcounts behind `realtime.Service`'s five Acquire/Release pairs (watcher, host watch, LSP, provider poll, origin sync); each resource keeps its start/stop body and its injected clock (the watcher's linger timer stays where it is today). |
| R10 | Entity payloads | Today's DTO shapes (S7 replaces `dto` with the wire file), minus placement: `AgentChatDTO.parentId/order` (`dto/agent.go`), `WorkspaceDTO.folderId/order`, `ChatWorktreeDTO.folderId/order`, `RepoDTO.order/folderId` (`dto/repo.go`) and the folder routes' `AgentChatDTO`-shaped folder replies (`{folder, shifted}`, `{chat, shifted}` in `endpoints/chat/handlers/folders.go`; there is no `FolderDTO`) go in T11. New: `TreeDTO{projectId, nodes: [{id, kind, parentId, order, name?}]}`, sent whole per project on every change (D2). `ProjectDTO.order` stays (D1). |
| R11 | Legacy placement translation (T3 → T11) | T3 changes the Tree's meaning while the client still speaks today's placement (`parentId: ""` for "the root of this scope", `folderId`, `order`). `usecases/tree/legacy.go` holds `ToLegacy` (the DTO fields) and `FromLegacy` (today's PATCH bodies) so the old routes keep their shapes. T11 deletes `ToLegacy`, the old routes and the DTO fields; `FromLegacy` stays because step 8 converts with it (engine R10: it goes when the step goes). |
| R12 | Migration | Two in-layout post-open steps at the next free numbers in `catalog.go`: S1 T12 took 6 (`StepChatLedger`, name `chat-ledger`, pre-open, impl in `adapter/migrations/chatledger`), so S4's are **7 and 8** unless something lands first. Step 7 `placement-tree` (T2) copies today's placement verbatim into one Tree per project and deletes `state/events/node.db`, `state/events/node_snapshots.db`, `state/store/node.db` and the `folders` table. Step 8 `tree-containers` (T3) rewrites every Tree to explicit containers with `FromLegacy`. Names are kebab-case like every catalog name. `RewritesState: true` is set on the step's `migrate.Plan` (it is a `Plan` field, not a `Step` field). Both run before any boot pass writes, read legacy facts once, and are idempotent per project (`Checkpoints.Mark`). |
| R13 | Reading legacy chat placement | From T2 the Chat aggregate no longer persists `ParentID`/`Order`: the fields stay on `domain.Chat` as read-time values tagged `json:"-"`, filled from the Tree by the chat usecase's placement read, until T4 deletes them once nothing reads them. In T2 the Chat aggregate gets `StripRetiredPlacementFields` as its next upcaster (the workspace precedent: replaying a `replace /order` patch against a struct without the field fails). Step 7 reads placement through a private read-only asynx instance over the chat event store typed `legacyChat` (`domain.Chat` embedded plus `ParentID string \`json:"parentId,omitempty"\`` and `Order int \`json:"order"\``), with the same schema version and upcasters as the app's Chat instance except a keep-all upcaster in place of the strip; no snapshot store, no subscribers, `Get` only; ids from the chat read model. Node rows are read the same way over `node.db` with a copied `legacyNode`. |
| R14 | IndexedDB | S4 deletes every reader and writer of the six `crowbar_*` entity stores and the seven `cache-store` stores (`entity-cache.ts`, `cache-store.ts`, `loadable-slice.ts`, `healEntityStores`, `maybeWipeOnVersionChange`, `wipeEntityCache`, `CROWBAR_CACHE_VERSION`). It keeps `crowbar` at version 11 (S5's `editor-drafts` bump), keeps the v1–v11 upgrade chain and the schema types it needs, and keeps the open-as-is fallback on `VersionError` (a production build may have bumped the database). `healEntityStores` reopens one version up when a store is missing, so deleting it is also what makes "S4 never changes the version" true. The stores stay on disk unread; removing them is S9's (amendment A7, D3). |
| R15 | Where code lives | Daemon: `api/internal/domain/tree`, `api/internal/domain/stream` (`go.domain`); `api/internal/app/repositories/tree` (`go.store`); `api/internal/app/usecases/tree` (`go.service`); `api/internal/app/stream` and `api/internal/api/v0/endpoints/stream` (`go.stream`, added to `modules.json` in T5); `api/internal/app/migratetree` (`go.app-misc`; not beside `adapter/migrations/chatledger`, because these steps read through the app's repositories, which `adapter` must not import). Client: `web/src/core/stream`, `web/src/core/entities` (`web.core`, prefix `web/src/core` added in T5); `web/src/features/workspace/tree` (`web.sidebar`, prefix added in T8). |
| R16 | Operations and ordering | Every command an operation's work sends uses `SendWait` (at refresh all 42 aggregate sends under `api/internal/app` already do; the three `.Send(` matches are provider driver sends, not aggregates), and the stream subscribes to every aggregate; `SendWait` returns after each subscriber was attempted, so every entity frame a piece of work produced is enqueued before the operation's terminal frame (one lock, one queue). From T10 `whenSettled` resolves on `succeeded` alone and `entity-presence.ts` (S3 R8's join) goes. |
| R17 | A chat's workspace | `Chat.WorkspaceID` stays stored in S4. The Tree refuses exactly what today's tree usecase refuses (a chat under a chat or workspace node of another workspace; R4), and allows what it allows (a chat filed into any folder keeps its bound workspace); the one resolver and the rebind that makes such a move legal are S8's (D6). |
| R18 | Row ids | Unchanged, because the persisted `sidebar-ui` record (`collapsedChatRows`) is keyed by them and a production build reads the same record: a workspace row's id is its owning chat's id when it has one, else the workspace id; a repo row's id is its home checkout's owning chat's id (`resolveHomeOwnerId`); folders and chats by their own id. T1 pins them. |
| R19 | Placement routes after T11 | `POST /v0/projects/:projectId/tree/moves {nodeId, parentId, index}` → 200 `{v, tree: TreeDTO}` (`v` from `hub.VersionOf` after `SendWait`; the client applies the reply as the entity frame it equals, so a drop never waits for the socket); `POST /v0/projects/:projectId/tree/folders {id, name, parentId, index}` → 201; `PATCH …/tree/folders/:id {name}` → 200; `DELETE …/tree/folders/:id` → 204 (its children move to its parent at its place). Moves finish inside the request and are not operations (S3 R2; D7). Refusals are `409` with one of `tree_cycle`, `tree_not_a_container`, `tree_cross_context`, `tree_cross_workspace`, `tree_fork_chain`, `tree_subtree_working`. |
| R20 | Wire contract | Declared once in `domain/stream` (Go) and mirrored by hand in `web/src/core/stream/frames.ts`; the client decoder rejects an unknown `t` or field by reporting it and dropping the scope (ledger D4 ruling, F22: a resubscribe would loop on a frame it can never decode). Generation is S7's (D4). |

## Decisions that change what the user sees

Ruled so the plan can proceed; the user may overturn any before its task.

| # | Change | Task |
|---|---|---|
| V1 | The sidebar paints from the daemon's snapshot at boot instead of the IndexedDB cache: a cold start shows the skeleton until the first snapshot (a local socket, typically tens of ms) instead of possibly stale rows. | T10 |
| V2 | A drag that the daemon refuses snaps back with the reason in a toast (one wording per refusal code) instead of a silent snap-back. | T11 |
| V3 | A dropped row lands where it was dropped in one step; the second re-render some drags show today while two aggregates settle is gone. | T11 |
| V4 | A move whose subtree holds a running chat is refused with "Stop the running chat before moving it" (today's guard, now worded). | T3 |

## Shared vocabulary

**Go `api/internal/domain/tree`** (T2 creates the types; T3 adds rules and verbs):

```go
type Kind string // "folder" | "repo" | "workspace" | "chat"
type Node struct {
	ID       string `json:"id"`
	Kind     Kind   `json:"kind"`
	ParentID string `json:"parentId,omitempty"` // "" = the project root
	Order    int    `json:"order"`              // dense index among ParentID's children
	Name     string `json:"name,omitempty"`     // folders only
}
type Tree struct {
	ProjectID string          `json:"projectId"`
	Nodes     map[string]Node `json:"nodes"`
}
type Facts struct {
	ProjectHome string            // the project home workspace id
	RepoHome    map[string]string // repo id → its home checkout's workspace id
	BoundTo     map[string]string // chat id → Chat.WorkspaceID, for chats in the subtree
	ForkParent  map[string]string // workspace id → Workspace.ParentID ("" = none)
	Working     map[string]bool   // chat id → working, for chats in the subtree
}
// T3: every verb returns a new Tree (no mutation) or one of the errors below.
func (t Tree) Children(parentID string) []Node // by Order, then ID
func (t Tree) Subtree(id string) []string      // id, then descendants depth-first in order
func (t Tree) Insert(n Node, parentID string, index int, f Facts) (Tree, error)
func (t Tree) Move(id, parentID string, index int, f Facts) (Tree, error)
func (t Tree) Rename(id, name string) (Tree, error)
func (t Tree) Remove(id string, promote bool) (Tree, []string, error) // promote: children take id's place
func (t Tree) Promote(chatID, workspaceID string) (Tree, error)       // chat node → workspace node, same place
func ResolveWorkspace(t Tree, id string, f Facts) string              // R4's walk
func Ancestors(t Tree, chatID string, ownerOf func(workspaceID string) string) []string // chat lineage, nearest first
type Placement struct{ ParentID string; Index int } // T12: where a create lands; carried on its operation item
var ErrCycle, ErrNotAContainer, ErrCrossContext, ErrCrossWorkspace, ErrForkChain,
	ErrSubtreeWorking, ErrNameRequired, ErrUnknownNode, ErrExists *problem.Error
```

**Go `api/internal/app/repositories/tree`** (T2, commands replaced in T3): `func New(ax asynx.Asynx[tree.Tree]) *Repository`; `Get(ctx, projectID) (tree.Tree, error)` (an absent aggregate is an empty tree); `Seed(ctx, t tree.Tree) (tree.Tree, error)`; T3: `Insert`, `Move`, `Rename`, `Remove`, `Promote`, `Replace(ctx, t tree.Tree)` (the tree-containers step only), each `(ctx, projectID, …) (tree.Tree, error)` sent with `SendWait`; `Forget(ctx, projectID) error`.

**Go `api/internal/app/usecases/tree`** (T3): `func New(repo *repotree.Repository, facts Facts) *Service` where `Facts` is the small read port the service needs (`Chat`, `Workspace`, `Repo`, `ProjectHome`, `Working`). Verbs: `Move(ctx, projectID, nodeID, parentID string, index int) (tree.Tree, error)`, `CreateFolder(ctx, projectID, id, name, parentID string, index int)`, `RenameFolder(ctx, projectID, id, name string)`, `DeleteFolder(ctx, projectID, id string)`, `Place(ctx, projectID string, n tree.Node, parentID string, index int)`, `Unplace(ctx, projectID, id string) ([]string, error)`, `Promote(ctx, projectID, chatID, workspaceID string)`, `Snapshot(ctx, projectID)`, `Ancestors(ctx, chatID string) ([]string, error)`. `legacy.go`: `ToLegacy(t tree.Tree, id string, f tree.Facts) (parentID string, order int)`, `FromLegacy(in LegacyPlacement, f tree.Facts) (tree.Tree, error)`.

**Go `api/internal/domain/stream`** (T5):

```go
type ScopeKind string // "projects" | "chat" | "ws" | "git" | "files" | "lsp" | "doc" (T6, OR2)
type Scope struct{ Kind ScopeKind; Key string } // "projects", "chat:<id>", …
func ParseScope(s string) (Scope, error) // unknown kind or missing key → problem.InvalidArgument
func (s Scope) String() string
type EntityKind string // "project" "repo" "workspace" "chat" "tree" "operation" "review_thread" "telemetry"
const (KindProject EntityKind = "project"; KindRepo = "repo"; KindWorkspace = "workspace"; KindChat = "chat"; KindTree = "tree"
	KindOperation = "operation"; KindReviewThread = "review_thread"; KindTelemetry = "telemetry") // typed EntityKind
var Projects = Scope{Kind: "projects"}
type Frame struct {
	T       string           `json:"t"`
	Scope   string           `json:"scope,omitempty"`
	Epoch   string           `json:"epoch,omitempty"`
	Kind    string           `json:"kind,omitempty"`
	ID      string           `json:"id,omitempty"`
	V       uint64           `json:"v,omitempty"`
	Data    json.RawMessage  `json:"data,omitempty"`
	Gone    bool             `json:"gone,omitempty"`
	Seq     int64            `json:"seq,omitempty"`
	Row     *ledger.Row      `json:"row,omitempty"`
	Delta   *ledger.FeedDelta `json:"delta,omitempty"` // T7 adds it: moves FeedDelta out of usecases/chat/internal/ingress (domain cannot import app)
	Reason  string           `json:"reason,omitempty"`
	Problem *problem.Problem `json:"problem,omitempty"`
}
type Request struct{ Op, Scope string; Since int64 } // json: op, scope, since
const QueueSize = 1024
const CloseResync = 4000
```

**Go `api/internal/app/stream`** (T5): `func New(epoch string, sources map[stream.ScopeKind]Source, resources map[stream.ScopeKind][]Resource, encode Encoder) *Hub`; `type Encoder func(kind stream.EntityKind, v any) (json.RawMessage, error)` maps a domain value to its wire payload (supplied by the API layer as the pure function `v0.EncodeEntity`, passed from the composition root into `app.New`, so `app` never imports `dto`); T11 adds `(*Hub).VersionOf(kind stream.EntityKind, id string) uint64` (the `v` of the last frame published for that key); `type Source interface { Attach(ctx context.Context, key string, since int64, emit func(stream.Frame) bool) error }` (an entity source emits its snapshot and returns; a ledger source keeps emitting until `ctx` ends); `type Resource interface { Start(key string); Stop(key string) }`; `(*Hub).Entity(scope stream.Scope, kind stream.EntityKind, id string, data any)`, `Gone(scope, kind, id)`, `Signal(scope stream.Scope, kind string, data any)`, `Serve(ctx context.Context, conn Conn) error`, `Close()`; `type Conn interface { ReadJSON(v any) error; WriteJSON(v any) error; CloseWith(code int, reason string) error }`.

**Web `web/src/core/stream`** (T5):

```ts
// frames.ts
export type Frame =
  | { t: 'ready'; epoch: string }
  | { t: 'entity'; scope: string; kind: EntityKind; id: string; v: number; data?: unknown; gone?: true }
  | { t: 'synced'; scope: string; seq?: number }
  | { t: 'row'; scope: string; row: LedgerRow }
  | { t: 'delta'; scope: string; delta: FeedDelta }
  | { t: 'signal'; scope: string; kind: string; data: unknown }
  | { t: 'closed'; scope: string; reason: 'resync' | 'deleted' | 'refused'; problem?: Problem }
export function decodeFrame(raw: unknown): Frame // throws FrameError on an unknown t or field
// stream.ts
export interface ScopeKindHandler { since?(key: string): number; frame(f: Frame): void; epoch?(epoch: string): void }
export function registerScopeKind(kind: string, h: ScopeKindHandler): () => void // a store registers its scope kind; core imports no feature
export function startStream(): () => void                     // main.tsx, once
export function holdScope(scope: string): () => void           // refcounted; sub on first hold, unsub on last release
export function onSignal<T>(scope: string, kind: string, cb: (data: T) => void): () => void
```

**Web `web/src/core/entities`** (T5 creates; T6, T10 add kinds):

```ts
export interface Versioned<T> { v: number; data: T }
export interface EntityState {
  epoch: string
  byKind: { [K in EntityKind]: Record<string, Versioned<EntityData[K]>> }
  members: Record<string, Record<string, true>> // scope → "kind:id" seen in its current snapshot
  synced: Record<string, true>
}
export const entityStore: StoreApi<EntityState>
export function useEntities<T>(selector: (s: EntityState) => T): T
export function applyEntityFrame(f: Extract<Frame, { t: 'entity' }>): void
export function applySynced(scope: string): void      // prunes members not seen since the scope's sub
export function resetForEpoch(epoch: string): void
// select.ts (T10): selectChat, selectWorkspace, selectRepo, selectProject, selectTree(s, projectId),
// selectChatsByWorkspace(s) (memoized on byKind.chat identity), selectReviewThreads(s, wsId) (T6)
```

**Web `web/src/features/workspace/tree`** (T8 creates; T10, T11 finish):

```ts
// tree-index.ts
export interface TreeNode { id: string; kind: 'folder' | 'repo' | 'workspace' | 'chat'; parentId: string; order: number; name?: string }
export interface TreeIndex { children(parentId: string): readonly TreeNode[]; node(id: string): TreeNode | undefined }
export function treeIndex(tree: TreeDTO): TreeIndex   // T10
// views.ts: the fields a row shows, and nothing else (T8 fills them from today's stores, T10 from entities)
export interface RowViews {
  chat(id: string): ChatView | undefined
  workspace(id: string): WorkspaceView | undefined
  repo(id: string): RepoView | undefined
}
// rows.ts
export interface TreeInput {
  projectId: string
  index: TreeIndex
  views: RowViews
  operations: readonly Operation[]   // S3 rows drawn from in-flight operation items (T12: at item.placement)
  ui: { removing: ReadonlySet<string> }
}
export function deriveRows(input: TreeInput): SidebarRow[] // the one row-derivation function
// input.ts: the only ways sidebar code obtains row data (T8 over today's stores, T10 over the entity store)
export function useTreeInput(projectId: string): TreeInput
export function treeInputFor(projectId: string): TreeInput
```

## Ownership after S4

| Fact | Owner after S4 | Today (deleted) |
|---|---|---|
| A sidebar row's parent and order | Tree aggregate (one per project) | Node aggregate, `Chat.ParentID/Order`, `folders` table, client fallbacks (folderId → fork parent → workspace ground) |
| A folder's name | its Tree node | `folders` table |
| Whether a move is legal | `domain/tree` rules | `usecases/chat/internal/tree/{validate,fork_chain}.go`, `app/tree`, client `sidebar-drop-policy.ts` (keeps only a mirror for the drop indicator, T11) |
| What a thread inherits | `tree.Ancestors` | `internal/lineage` over chat rows |
| Every daemon → client fact | `app/stream.Hub`, one socket | 9 stream defs (incl. S5's `documentsDef`), S1 feed, S3 operations stream, `hub` (33 methods), `snapshots.go` |
| Watchers, host-document watches, LSP hosts, forge polls, origin sync | `app/stream` refcount per scope | `realtime.Service`'s five Acquire/Release pairs |
| Server truth on the client | `core/entities` store (+ ChatStore `feeds` for ledgers) | IndexedDB entity cache, `cache-store`, `loadable-slice`, `app-sync-engine`, nine `lib/store` stores, `ChatStore.chats/byWorkspace/scopes/telemetry` |
| Sidebar rows | `deriveRows` over Tree + entities | `rows-from-repo`, `rows-from-home`, `buildSidebarTree`, `build-repo-tree`, `repo-tree`, `rows-from-operations` |
| Sidebar UI state | `features/workspace/tree/tree-ui-store.ts` | `sidebar-selection`, `sidebar-inline-rename`, `sidebar-removal`, `sidebar.ts` UI slice, `create-draft` |
| A restored pane's chat workspace | `selectChat(id).workspaceId` | `placeRestoredChatMembers` GETs |

## Deletion table (S4 "Deletes" → task)

| Deleted | Task |
|---|---|
| Node aggregate (`app/repositories/node/`, incl. its `internal/store/hub.go`; `domain/node.go`'s `Node`/`NodeKind`), its three database files, `folders` table and `domain.Folder`, persisted `Chat.ParentID/Order` (the fields go in T4), chat `set_placement`/`set_order` commands, `home_read.go`'s Node overlay, `owning_chats_reconcile.go`'s placement half | T2 |
| `app/tree/`, tree usecase planning (`plan.go`, `walk.go`, `tree_snapshot.go`, `forest_scope.go`, `home_forest.go`, `legacy_rows.go`, `write_node.go`, `home_ports.go`, `validate.go`, `fork_chain.go`, `place_workspace.go`, `subtree.go`, `guard_not_working.go`) and T2's `Put`/`Delete` Tree commands | T3 |
| `usecases/chat/internal/tree/` (whole package incl. `internal/lineage`), `domain.Chat.ParentID/Order`, `usecases/worktree/chat_forest.go` placement reads, `placement_reads.go`, T2's placement ports adapter | T4 |
| `/v0/operations` WebSocket, `operationsDef`, `operations-stream.ts`, `hub.BroadcastOperation/PushOperation` (all *(S3 T3)*) | T5 |
| `gitDef`, `filesDef`, `lspDef`, `threadsDef`, `terminalsDef` (no client consumer), `documentsDef` and `withHostWatchLifecycle` (OR2), `withWatcherLifecycle/withLSPLifecycle/withOriginSyncLifecycle`, `app/realtime/`, their WS routes (incl. `endpoints/documents` `/documents/ws`), hub methods and client subscriptions (incl. `features/workspace/lib/external-buffer-sync.ts`'s socket) | T6 |
| `/v0/chats/:chatId/feed` WebSocket handler (`endpoints/chat/handlers/feed.go`, mounted in `api/v0/router.go`), `wsManager.subscribe`'s `url` parameter *(S2 T3)* | T7 |
| `rows-from-repo.ts`, `rows-from-home.ts`, `rows-for-project.ts`, `workspace-tree-utils.ts`'s `buildSidebarTree`, `build-repo-tree.ts`, `rows-from-operations.ts` *(S3 T5)* | T8 |
| `sidebar-selection.ts`, `sidebar-inline-rename.ts`, `sidebar-removal.ts`, `sidebar.ts` UI slice, `create-draft.ts` *(S3 T5)*, `sidebar-ui.ts` store (persistence helper stays) | T9 |
| `projectsDef`, `reposDef`, `agentChatDef` (`/chats/ws`), home tree WS, `api/v0/ws/`, `app/hub/`, `api/v0/snapshots.go`; `app-sync-engine.ts`, `app-sync-provider.tsx`, `entity-cache.ts`, `cache-store.ts`, `loadable-slice.ts`, `entity-stream.ts`, `structural-chat-folder-frame.ts`, `chat-frame-kinds.ts`, `home-tree.ts`, `folder-signal.ts`, `sidebar.ts`, `sidebar-sync.ts`, `projects.ts`, `project-visibility.ts`, `workspace-list.ts`, `repo-tree.ts`, `use-workspace-provider-stream.ts`, `legacy-tree-index.ts`, `entity-presence.ts` *(S3 T3)*, `placeRestoredChatMembers`, `healEntityStores`, `maybeWipeOnVersionChange`, ChatStore `chats/byWorkspace/scopes/telemetry` *(S2 T4)* | T10 |
| `ToLegacy`, `PATCH …/chats/:id/placement` (repo, home), `PATCH …/workspaces/:id/placement`, `…/chats/folders` CRUD (repo, home), repo PATCH `order`/`folderId`, DTO placement fields, the folder routes' `AgentChatDTO` replies; client `lib/api/sidebar-placement.ts`, `applied-placement.ts`, `drop-actions.ts`'s placement planning | T11 |
| `create-actions.ts`, `thread-create.ts`, `home-actions.ts`, `row-actions.ts`, `removal-plan.ts`, `removal-commit.ts` (replaced by `features/workspace/tree/{create,remove,actions}.ts`) | T12 |
| Stale `modules.json` prefixes; every remaining `components/sidebar`, `components/tree-dnd`, `lib/store` file (moved or deleted) | T13 |

## Module keys and counters

Baselines are `loc-budgets-baseline.json`; "measured" is `node scripts/ratchet/loc-budgets.mjs` at refresh (`d8404d2e`, read-only).

- `go.stream` (baseline 3,278, measured 3,572: S5's documents stream and host watch): T5 adds `api/internal/app/stream` and `api/internal/api/v0/endpoints/stream`; T6 and T10 delete `realtime`, `hub`, `ws`, `snapshots.go`. Expected after S4: about 1,300 (hub, connection, sources, resources).
- `go.store` (baseline 6,921, measured 8,776): loses Node (≈1.1k), `app/tree`, OCC remnants; gains `repositories/tree` (≈300). `go.chat` (baseline 38,317, measured 32,230) loses the tree usecase (S1 gate measure: 4,990 source, 4,789 tests). `go.domain` (baseline 2,025, measured 4,090 after S1's `domain/ledger`) gains `domain/tree` (≈600) and `domain/stream` (≈150). `go.dto` (baseline 2,165, measured 2,171) loses the placement fields, gains `TreeDTO`.
- `web.sidebar` (baseline 18,562, measured 18,566): T8 adds `web/src/features/workspace/tree`. T13 moves files whose owner is not the sidebar to their module (D9 lists them) and moves their budget with them. Target ≤ 5,000.
- `web.core` (baseline 5,979, measured 6,058): T5 adds `web/src/core`; loses `app-sync-engine`, `lib/ws/entity-stream.ts`, `lib/persistence/{entity-cache,cache-store}.ts`, `lib/api/sidebar-placement.ts`.
- Counters: `web.timers` falls (`SUBSCRIPTION_GRACE_MS`, `REBUILD_BATCH_MS`, reseed loops); `go.timers` unchanged (the watcher linger and forge polls move, not change). T13 writes the measured values.

## Gate (§11 S4) and how T13 measures it

| Gate | Baseline | Target | Measure |
|---|---|---|---|
| Sidebar LOC | `web.sidebar` 18,562 (S0; 18,566 measured at refresh) | ≤ 5,000 | `node scripts/ratchet/loc-budgets.mjs` |
| Stream definitions | 9 at refresh (+ S1 feed, + S3 operations once S3 T3 lands) | 1 | `grep -rn 'websocket.Upgrader' api/internal --include='*.go' \| grep -v _test` lists exactly the stream, the terminal PTY socket and the console log tail (at refresh: `ws/client.go`, `terminal/handlers/ws.go`, `console/handlers/logs.go`; the feed and documents sockets upgrade through `ws`); `ws.StreamDef` has no match |
| Aggregates | 4 at refresh (ReviewThread, Workspace, Chat, Node) | 4 (Workspace, Chat, ReviewThread, Tree) | `grep -rn 'newAsynx\[' api/internal/app` |
| Client polls | 3 periodic + 4 edge before S2; the grep finds 17 lines at refresh | 0 reseed GETs | `grep -rn 'fetchRepos\|fetchHomeChats\|fetchRepoChats\|fetchProjects' web/src --include='*.ts*' \| grep -v __tests__` finds only user-initiated reads |
| IndexedDB entity cache | 6 + 7 stores read and written | 0 read or written; database untouched | T10 tests and T13 live check |

## Amendments to earlier plans

The earlier plan files are not edited; these notes override them where they meet S4.

- A1 (S1 R8, T10): the per-chat feed WebSocket goes in T7; `OpenFeed` stays as the `chat:` scope's source, its frames mapped by R6 (`ready{seq}` → `synced{seq}`, `resync` → `closed{resync}`, `deleted` → `closed{deleted}`; the epoch moves to the connection's `ready`).
- A2 (S2 R15–R17, T4, T11): a chat feed attaches with `holdScope('chat:<id>')` (T7) under R17's lifetime rule; `wsManager.subscribe`'s `url` parameter goes. R16's lifecycle subscriptions go in T10; `ChatStore` keeps `feeds` and `outbox` only, and its `chats`, `byWorkspace`, `scopes` and `telemetry` move to the entity store (telemetry as entity kind `telemetry` on the chat scope, so the GET on attach goes). R21's command-URL unification is S7's route consolidation, not S4's.
- A3 (S3 R7, R8, D3, A7): operations ride the `projects` scope from T5 (`entity` frames of kind `operation`, applied by the operations store's existing `applyOperationFrame`). R8's presence join goes in T10 (R16). Rows drawn from operation items take their place from the create request's Tree position (T12).
- A4 (S3 R16): `lib/store/{operations,run-operation}.ts` move to `web/src/core/entities/` in T10; `entity-presence.ts` is deleted; `create-draft.ts` folds into the tree UI store in T9.
- A5 (S5): `hydrate.ts` loses `placeRestoredChatMembers` and `hydrateSidebar`'s store write in T10; the per-workspace store's git, files and threads slices are fed from the stream after T6 (threads as entity selectors). S5 follows the responsibility.
- A6 (S8): the one resolver for a chat's workspace and the rebind that makes a cross-workspace move legal are S8's (R17). Steps 7 and 8 (placement-tree, tree-containers) and `FromLegacy` fold into S8's database merge. The Tree's event and snapshot files join S8's one database.
- A7 (S9): delete the 13 unread IndexedDB object stores and `crowbar:cache-version` with a coordinated version bump once no installed build reads them (R14); the per-workspace store registry and its remaining slices.
- A8 (S7): generated client types for the wire (R20, D4); `dto` deletion includes `TreeDTO` and the entity payloads.
- A9 (migration engine): two post-open, in-layout catalog steps (R12); their impls live in `api/internal/app/migratetree` and register beside `legacyImpls`.

## Review Focus

The failure modes most likely to bite a user that no task's main tests exercise, each pinned by a named test in its owning task:

1. An upgraded home must show exactly the hierarchy it showed before, including today's ambiguous cases: a folder written before `HomeID` existed, a forked workspace filed in no folder, a thread under a branch's owning chat, a repo filed in a home folder, two siblings with equal `order` (T2 `TestPlacementTreeStep_CopiesEveryLegacyPlacement`; T3 `TestTreeContainersStep_KeepsTheRenderedHierarchy`; T13 live before/after outline diff).
2. A reconnect while a turn streams must neither duplicate nor drop a row, and an entity deleted while the socket was down must disappear (T5 `stream.test.ts` "a resubscribe snapshot removes an entity deleted while offline"; T7 `TestStream_ChatScopeResumesFromSinceWithoutGapOrDuplicate`).
3. A daemon restart while the window stays open must not leave the client ignoring the new daemon's lower versions (T5 `entity-store.test.ts` "a new epoch replaces every entity even at a lower version"; T5 `TestStream_ReadyCarriesANewEpochAfterRestart`).
4. An import that succeeds must show its rows the moment it says so; the operation's `succeeded` frame must never overtake its entities (T10 `TestStream_OperationSucceededFollowsItsEntities`).
5. A production build sharing the `crowbar` IndexedDB with an S4 build must find its database exactly as it left it (T10 `idb-version-skew.test.ts` "an S4 build opens a v11 database without changing its version or its stores").

## Spec issues found while planning

1. §4 "attach live before snapshot, dedupe by seq" fits ledgers, which have a `seq`; entities do not. R7 gives entities the stream's publish sequence and computes snapshots under the publisher's lock, which is equivalent and needs no per-entity durable version.
2. §3 "A create is one Tree write plus one entity write, published together with one operation": they are two asynx aggregates, so two commits. R16 makes them published together (ordered before the operation's terminal frame); atomicity across them waits for S8's single database, and a create whose Tree insert fails discards its entity (T12).
3. §11 S4 "aggregates 6 → 4" was already met by S1 (runner and activity deleted). S4 replaces Node with Tree, so the count stays 4.
4. §4's generated, CI-diffed wire contract overlaps S7's "one `wire` file" (R20, D4).
5. "Nine `lib/store` sidebar stores" (§8, review) names `sidebar-sync` and `pending-creates`, while today's directory also holds `projects`, `workspace-list`, `home-tree`, `repo-tree`, `applied-placement`, `project-visibility`. S4 empties `lib/store` of everything sidebar (Deletion table); S3 already deleted `pending-creates`.
6. `Chat.ParentID` is also what a thread reads (lineage). After S4 lineage walks the Tree: chat ancestors through folders, a workspace node counting as its owning chat (`tree.Ancestors`).
7. §8 deletes "the per-workspace store registry and its seven slices" but §11 does not give it to S4. S4 changes only their transport (A5, A7).
8. S2 R21 says unifying chat command URLs is S4's; §11 gives route consolidation to S7 (A2).

## Decisions (user 2026-10-18: "Agreed with everything", with the ledger's amendments noted on each)

- **D1. Project order stays on the project.** Spaces (one per project) are not Tree rows; `Project.Order` and `PATCH /v0/projects/:id {order}` stay. Recommendation: keep. Cost if wrong: a root Tree aggregate over projects plus one migration step, about one task.
- **D2. A Tree frame carries the whole project tree.** Recommendation: whole tree (a few KB to ~100 KB per move on a large project, over a local socket). Cost if wrong: node-level deltas later change one frame payload and one reducer.
- **D3. The 13 unread IndexedDB stores stay on disk until S9.** Recommendation: yes; deleting them needs a version bump that a production build on the same database would then fight. Cost if wrong: none to users; S9 deletes them with a coordinated release.
- **D4. Generated client types wait for S7.** Recommendation: S4 hand-mirrors the frame envelope with a strict decoder. Cost if wrong: a generator now (about one task) that S7 rewrites for the whole wire. **Ruled (ledger):** as recommended, but an undecodable frame drops its scope instead of resubscribing (F22; R20).
- **D5. The `projects` scope carries every project's entities.** Recommendation: all projects at boot (the carousel switches spaces without a load). Cost if wrong: a very large home pays a larger first snapshot; T13 records its size and time; per-project scopes (`project:<id>`) are additive. **Ruled (ledger):** adopted "only after the F4 fix"; F4's text is in the lost pre-flight (OR3).
- **D6. Moves that change a chat's workspace stay refused until S8** (R17), as today. Cost if wrong: S8's rebind is the only safe way to move a live conversation's folder. **Ruled (ledger):** refuse only what is refused today (chat-under-chat across workspaces); filing a chat into a folder of another workspace stays allowed (R4, R17).
- **D7. Tree moves answer inside the request (200), not as operations.** Recommendation: yes (S3 R2). Cost if wrong: S8 turns cross-workspace moves into operations with the rebind anyway. **Ruled (ledger):** adopted "with the F6 fix"; F6's text is in the lost pre-flight (OR3).
- **D8. Entity versions are the stream's publish sequence per daemon epoch** (R7), not durable per-entity versions. Cost if wrong: resuming entity scopes without a re-snapshot would need durable versions, which S8's one database can provide.
- **D9. Files under `components/layout` whose owner is not the sidebar leave `web.sidebar`** in T13 with their budget: `ide-shell.tsx`, `ide-shell-workspace-content.tsx`, `use-ide-shell-workspace-retention.ts`, `use-publish-focused-workspace-context.ts`, `nav-stack.tsx`, `fps-overlay.tsx`, `connection-indicator.tsx`, `workspace-switcher.tsx`, `workspace-switcher-model.ts`, `context-pill-model.ts`, `format-change-count.ts`, `use-redirect-if-project-missing.ts`, `repo-import-dialog.tsx`, `icon-popover.tsx`, each named by its file path under `web.app-shell` (shell, dialogs) or `web.workspace-panes` (workspace switcher, context pill) in `modules.json`; no file moves. Recommendation: yes; the 5k gate then measures the tree and its chrome. Cost if wrong: if the user counts these as sidebar, the gate needs about 1.5k more lines of cuts that this plan does not find without removing features. **Ruled (ledger, F12), overriding the list above:** re-key only the shell files (842 lines), delete the dead files (427), and keep the sidebar-owned files (784) in `web.sidebar`; re-keying sidebar-owned code to meet the gate is loosening a budget. The ≤5k gate stands (F11): if real deletions cannot reach it, the counts go to the user as a spec question, never a budget move. T13 classifies each listed file (shell, dead, sidebar-owned) by its importers before moving anything.
- **D10. The terminal-list stream is deleted without replacement** (no client subscribes to it). Recommendation: delete. Cost if wrong: a `terminal` entity kind on the `ws:` scope, about 60 lines.

## Open rulings (refresh 2026-10-28)

Each needs the controller's or the user's ruling before the task it names is dispatched.

- **OR1. S4 runs in two waves, not after all of S2.** T1–T5 need S1, S3 T1, S3 T3 and S3 T8 but nothing of S2 (Dependencies); T6–T13 need S2 T1, T3, T4 and T11. Recommendation: dispatch T1 as soon as the S1 final review lands, T2–T4 after S3 T8, T5 after S3 T3, in a lane beside S2; hold T6 until S2 T11 merges. Cost if wrong: the shared Go files (`dto/agent.go`, `endpoints/chat/handlers/chats.go`, `api/v0/container.go`) conflict on rebase; the controller's min-merge and a re-run of T1's pin cover it.
- **OR2. S5's host-document stream gets a scope.** `/v0/documents/ws` (`documentsDef`, `withHostWatchLifecycle`, `realtime.AcquireHostWatch`) is keyed by a host path, not a workspace, and refuses an upgrade with 507 when the OS has no watches left. Recommendation: T6 adds scope kind `doc:<url-escaped absolute path>`; `Resource.Start` returns an error, and a failed start closes the scope `refused` with a problem whose code tells the watch limit apart (the client keeps S5's wording). Cost if wrong: if the documents socket stays, the gate's "1 stream definition" fails and the hub keeps a sixth refcount outside it.
- **OR3. The S4 pre-flight is lost.** The ledger adopts F2–F32 of `preflight.md`, which was not carried to the cloud; only F1, F11, F12, F21 and F22 survive through the ledger's text, and D5/D7 hang on F4/F6. Recommendation: before T1 is dispatched, an opus pre-flight of this refreshed plan regenerates the findings; the controller rules on any that contradict the user-accepted decisions. Cost if wrong: the plan ships without up to 27 findings the user already accepted.
- **OR4. The live "before" needs the user's machine or the noVNC container.** T1 Step 5's `home-before.tar` and outline are T13's upgrade fixture, so T2 must not merge before they exist. Recommendation: run T1 Step 5 in the noVNC container (isolated `CROWBAR_HOME`, the S2 R25 fake provider), as the S5 and S6 live gates did; T13 Step 4 runs there too. Cost if wrong: T13 has no pre-S4 home to upgrade, and Review Focus 1 rests on the unit tests alone.
- **OR5. T1's pin test file already has four `vi.mock`.** `sidebar-tree-surface.test.tsx` mocks the router, the workspace store registry, the toast store and the home-workspace resolver (CLAUDE.md: ≤3). Recommendation: T1 brings it to three by replacing the `home-workspace-resolver` mock with a real home fixture loaded by `loadPinFixture`, in the same commit. Cost if wrong: every later task extends a file that breaks the rule.

## Refresh log (2026-10-28, against `d8404d2e`)

Stale items fixed in place across the index and s4a–s4d: asynx state (P1); disk-check path and the Go test form (P3, Global Constraints, macOS paths removed); `make lint`/`make deadcode`/`make pr-checks`/`make ratchet` replaced by the handoff's direct commands; migration step numbers 6/7 → 7/8 (S1 took 6) and `RewritesState` placement; IndexedDB v10 → v11, store list re-derived (still 17); stream definitions 8 → 9 (S5 `documentsDef`); `realtime` four → five refcounts and its real file list (T6); `FolderDTO` (does not exist: folders reply as `AgentChatDTO`); `ChatDTO` → `AgentChatDTO`; the R16 `.Send(` grep (all aggregate sends are already `SendWait`); the feed route's real mount (`router.go`, not `chat/routes.go`); the Workspace aggregate's publisher path (`repositories/workspace/internal/store/projections/hub.go`); missing web test files that T6 said to extend (it creates them); measured module and gate numbers; paths that only S2/S3 create marked *(S2 Tn)*/*(S3 Tn)*; the ledger's F1/D4/D6/D9/F11/F21/F22 rulings applied to R4, R17, R20, D-entries, T3 and T13; oversize files each task edits named with their sizes.
