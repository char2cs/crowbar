# Slice 1a — the sidebar

**Date:** 2026-08-07
**Status:** approved by the user, ready for an implementation plan
**Implements:** [`2026-08-04-slice-based-port-method-design.md`](2026-08-04-slice-based-port-method-design.md)
§4 row 1a, widened to absorb rows 1a's mutation and drag halves.
**Architecture:** [`2026-07-30-rust-native-desktop-port-design.md`](2026-07-30-rust-native-desktop-port-design.md)
§4.2 (crate graph), §4.3 (invariants), §12 (coverage) — all unchanged and all
still enforced.

---

## 1. The objective, stated as a test

**A user cannot tell the Rust sidebar from the Tauri sidebar**, visually or
functionally, except where §13 records an accepted delta. Both apps run against
one `CROWBAR_HOME`, side by side, and the screenshot pair is the evidence.

That is the *whole* sidebar: the resizable panel and its chrome, the four-panel
carousel, the workspace tree with real streaming daemon data, and every
interaction the React sidebar offers — selection, expand/collapse, rename,
create, import, detach, drag-to-reparent, drag-to-trash.

## 2. What is already done, and why this slice is not a porting job

The sidebar's **views are already ported.** Of the 28 components `IDEShell`
mounts inside the sidebar subtree, 26 live in `crowbar-ui` today, each
individually parity-checked under the retired oracle. They have **never been
assembled.**

| React component | `crowbar-ui` |
|---|---|
| `SidebarProvider` / `ui/sidebar.tsx` | `surfaces/sidebar/shell.rs` |
| `SidebarPeek` | `surfaces/sidebar/sidebar_peek.rs` |
| `SidebarProjectHeader` | `surfaces/sidebar/sidebar_project_header.rs` |
| `SidebarTabBar` | `surfaces/sidebar/sidebar_tab_bar.rs` |
| `SidebarCarousel` | `surfaces/sidebar/sidebar_carousel.rs` |
| `SidebarToastOverlay` | `surfaces/sidebar/sidebar_toast_overlay.rs` |
| `SidebarSkeleton` | `surfaces/sidebar/sidebar_skeleton.rs` |
| `SidebarToggleIcon` | `surfaces/sidebar/sidebar_toggle_icon.rs` |
| `NavStack` | `surfaces/sidebar/nav_stack.rs` |
| `ContextPill` | `surfaces/context_pill.rs` |
| `WorkspaceTree` | `surfaces/workspace/workspace_tree.rs` |
| `WorkspaceTreeItem` | `surfaces/workspace/workspace_tree_item.rs` |
| `WorkspaceSwitcher` | `surfaces/workspace/workspace_switcher.rs` |
| `WorkspaceInlineInput` | `surfaces/workspace/workspace_inline_input.rs` |
| `WorkspaceBranchIcon` | `surfaces/workspace/workspace_branch_icon.rs` |
| `RepoSection` | `surfaces/repo/repo_section.rs` |
| `RepoAvatar` | `surfaces/repo/repo_avatar.rs` |
| `RepoIconPopover` | `surfaces/repo/repo_icon_popover.rs` |
| `RepoImportDialog` | `surfaces/repo/repo_import_dialog.rs` |
| `ProjectHomeRow` | `surfaces/rows/project_home_row.rs` |
| `ProjectSwitcherPanel` | `surfaces/rows/project_switcher_panel.rs` |
| `PendingCreateRow` | `surfaces/rows/pending_create_row.rs` |
| `PlaceholderRowActions` | `surfaces/rows/placeholder_row_actions.rs` |
| `DetachHolderModal` | `surfaces/detach_holder_modal.rs` |
| `ScrollArea` | `primitives/scroll_area.rs` |
| `InlineError` | `primitives/inline_error.rs` |
| **`WorkspaceTreeFooter`** | ❌ **built here** |
| **`ConnectionIndicator`** | ❌ **built here** |

The two missing ones:

* `workspace_tree_footer` — the drag-to-trash target. 44 TS lines, rendered
  always (never mounted/unmounted, so the `ScrollArea` does not resize on drag
  start) and revealed by a `max-height` transition when a drag begins.
* `connection_indicator` — the daemon-connection state readout.

Three ported components carry a known defect from the archive and are fixed as
they are composed, not worked around: `repo_import_dialog` is **REFUSED** by the
oracle for a duplicate `button` anchor id, `project_switcher_panel` reports
5 anchors of which only 1 is real, and `repo_avatar` reports 4 of which only 1
is real.

So this slice is **state, transport and composition**. `crowbar-state` is
currently 13 lines of doc comment and zero code; `crowbar-client` has one verb.

## 3. Inherited constraints — not re-decided here

1. **`crowbar-core` never mentions `gpui`.** check-invariants rule 1.
2. **Design tokens are sealed newtypes.** A colour or spacing literal outside
   `crowbar-ui/src/theme/` does not compile. Every value in this slice is
   `theme.*`.
3. **`unsafe` only in `crowbar-platform`**, with a `# Safety` comment proving the
   obligation. Every other crate root is `#![forbid(unsafe_code)]`.
4. **`unwrap`/`expect`/`panic!`/`todo!`/`unimplemented!` denied outside tests**,
   plus all of `clippy::pedantic`. No per-site `#[allow]`.
5. **≥98% line coverage on the logic crates.** Everything this slice puts in
   `crowbar-core` is under that gate, which is the reason the pure half is
   pushed down there aggressively.
6. **Menus are native `NSMenu`** — the user's decision of 2026-07-31, recorded
   in `native/oracle/blocked/s13-native-menus-accepted-delta.md` and formalised
   as spec §13 entry 7 in `a4de9aef`. See §11.
7. **`gpui` does not run a tokio reactor.** `reqwest` is pinned `blocking` for
   exactly this reason. The WS transport in §5 obeys the same constraint.

## 4. Architecture

```
daemon
  │  GET  /v0/projects
  │  GET  /v0/projects/:p/repos
  │  GET  /v0/projects/:p/repos/:r/workspaces         ── seed
  │  WS   (the same three paths, upgraded)            ── stream
  ▼
crowbar-client        unix-socket HTTP + WS · handshake · reconnect · backoff
  │                   frames out on a channel; no domain knowledge
  ▼
crowbar-state         Entity<EntityCache>  { projects, repos, workspaces }
  │                   one subscription task per open scope
  ▼
crowbar-core          build_scoped_repo_tree() · build_workspace_tree()
  │                   SidebarPanel · SidebarTabs · CollapseSet · WorkspaceScopeRegistry
  ▼
crowbar-state         Entity<SidebarStore>  { repos, collapsed, active_tab, scope, drag }
  ▼
crowbar-ui            the 29 surfaces + 2 new ones, composed
  ▼
crowbar-app           the shipping window: resizable group · sidebar · blank content
```

**No persistent cache.** React writes DTOs to IndexedDB (`crowbar_repos`,
`crowbar_workspaces`) so a page reload survives. A native process has no reload,
and `QUEUE.md` records that same browser cache defeating `CROWBAR_HOME`
isolation because the origin is shared between dev instances. DTOs live in
`Entity<EntityCache>` and die with the process; a restart re-seeds over HTTP.
The *ordering* discipline the cache enforced is kept — see §5.4.

**No router.** React derives the active project/repo/workspace from the TanStack
URL `/ide/:projectId/:repoId/:wsId` and calls `recordWorkspaceScopeFromPath`
synchronously during render. `crowbar-core::workspace::scope::WorkspaceScopeRegistry`
is already ported and already carries `active_workspace_id`; it goes behind one
`Entity<T>` and a row click sets it. Nothing parses a path.

## 5. The transport — `crowbar-client`

### 5.1 One endpoint, two verbs

The daemon **dual-serves**: `GET /v0/projects/:p/repos/:r/workspaces` returns the
JSON list *and* accepts a WebSocket upgrade on the same path. The dedicated
`/ws/workspaces`, `/ws/git`, `/ws/files`, `/ws/lsp` and `/ws/terminals/:id`
routes were folded into this in W7-2 and their absence is asserted by
`api/internal/api/v0/route_audit_test.go:338`. Seed with the GET, upgrade the
same path — the React `subscribeEntityStream` contract, reproduced.

### 5.2 Sync WebSocket on an owned thread

`tungstenite` (sync), **not** `tokio-tungstenite`: §10.1 pins `reqwest` to
`blocking` because GPUI runs no tokio reactor, and introducing one for the
sidebar would be a second runtime in a process that has none.

* `std::os::unix::net::UnixStream::connect(socket_path)`.
* `tungstenite::client::client(request, stream)` performs the RFC 6455
  handshake over that stream — it accepts any `Read + Write`.
* One OS thread per channel, blocking `read()` loop.
* Frames leave on a `smol` channel. `crowbar-state` awaits them in a
  `cx.background_spawn` task and applies them with `cx.update`.

The thread is the unit of teardown: dropping the channel handle closes the
socket, the blocking read returns, the thread exits. No cancellation token, no
reactor.

### 5.3 Reconnect and backoff — the React numbers, exactly

Base delay **1000 ms**, doubling, capped at **30 000 ms**, reset to base on a
successful open (`web/src/lib/ws/manager.ts`). On reconnect the subscriber
receives a **reconnect sentinel**, and a sentinel triggers a **full GET reseed**
— frames missed during an outage can never be recovered by merging a single DTO.

Two behaviours from the React manager that are bugs if dropped:

* A channel whose subscriber set has emptied is **not** resurrected on the
  reconnect timer; it is removed.
* The doubled delay is **carried into** the replacement channel, so repeated
  failures actually back off instead of restarting at the base.

### 5.4 Frame ordering is load-bearing

React serialises every cache mutation onto one promise chain because a `new`
then `deleted` pair for the same id could otherwise commit out of order and
resurrect a tombstoned row (recorded as H21). Rust gets this for free from a
single-consumer channel applied on one task — **provided the seed is applied on
that same task**, not raced beside it. It is.

A **seed generation counter** is kept: a reseed bumps it, and an in-flight seed
whose generation is stale discards its result rather than writing it.

### 5.5 `prune_scope` — the sibling-wipe bug, ported deliberately

`crowbar_workspaces` is fed by two streams: the per-repo list (seed = all of a
repo's workspaces) and the per-`wsId` stream (seed = one workspace). Without a
scope predicate, the single-workspace seed is treated as authoritative over the
whole store and prunes every sibling — navigating into one workspace deleted all
the others from the sidebar until reload. Every stream declares its own scope:
`|ws| ws.repo_id == repo_id` for the list, `|repo| repo.project_id == project_id`
for the repo list.

### 5.6 Stream lifecycle on project switch

Tear down the previous project's streams **before** seeding the new one, and
re-scope the tree immediately so the old project's repos leave the screen at
once rather than lingering until the seed resolves. React does this in
`app-sync-provider.tsx`; the ordering is the whole point.

The project-home workspace rides no repo, so no per-repo stream can ever carry
it. It is tracked at project scope by its own subscription, or the home row's
`working` spinner never turns.

## 6. `crowbar-core` — the pure half

Everything here is plain Rust under the ≥98% gate. No `gpui`, no I/O.

| module | ported from | what it owns |
|---|---|---|
| `sidebar::tree` | `build-repo-tree.ts` | `build_repo_tree`, `build_scoped_repo_tree`, `to_sidebar_repo`, `to_sidebar_workspace`; avatar label + colour derivation |
| `sidebar::hierarchy` | `workspace-tree-utils.ts` | `build_workspace_tree` — parent→child nesting **with cycle detection**: a node whose ancestor chain revisits itself is promoted to a root rather than dropped or looped |
| `sidebar::panel` | `use-sidebar-panel.ts` | width clamp, open/collapsed, and the window-vs-drag discrimination in §6.1 |
| `sidebar::tabs` | `sidebar.ts`, `sidebar-carousel.tsx` | the four-tab enum, active tab, index↔offset mapping |
| `sidebar::collapse` | `sidebar.ts` | collapsed repo and workspace id sets |
| `sidebar::drag` | `workspace-tree-context.tsx` | reparent legality — cycle rejection, self-drop, locked rows |

`WorkspaceScopeRegistry`, `find_workspace_for_branch`, placeholder classification
and keep-alive policy are **already** in `crowbar-core::workspace` and are
consumed, not rewritten.

### 6.1 The width rule, reproduced exactly

`SIDEBAR_MIN_PX = 250`, `SIDEBAR_MAX_PX = 640`, `SIDEBAR_DEFAULT_PX = 294`.
Stored width is **clamped at both ends** on load — the peek card takes the raw
number, so a stale over-large value would render past the opposite window edge.

**Only a separator drag expresses a width preference.** Everything else that
resizes the sidebar is a consequence of something else, and persisting those
indiscriminately let one narrow window redefine the width permanently: at a
700 px window the content pane's 20% minimum squeezes a 640 px sidebar to
559 px, and 559 was then stored as if chosen.

The discrimination is a **latch, not a per-event test**, because one window
resize emits **two** resize events at the same group width — first the existing
percentage applied to the new width, then `preserve-pixel-size` correcting it
back to pixels (measured on a 1714→700 narrowing: 245 px at 35%, then 559 px at
80%). Only the first carries a changed group width, so a per-event test lets the
second through as if the user had chosen it. A pointer-down clears the latch; a
changed group width sets it. **Note which way it fails:** a missed pointer-down
means a width is *not remembered*, never that a squeezed one is.

The chosen width is held in a pending slot during the drag and **committed
once**, on settle. React learned this the expensive way: a state write per frame
re-rendered the shell and a synchronous `localStorage` write blocked it, ~90
times per drag.

### 6.2 Persistence goes daemon-side

`sidebar-open` and `sidebar-width` are the two un-namespaced `localStorage` keys
`QUEUE.md` flags; the daemon already stores them as `global` rows. The Rust app
reads and writes **the same rows the React app does**, so both apps come up at
the same width against one `CROWBAR_HOME` — which is what makes the side-by-side
capture honest rather than a coincidence of defaults.

## 7. `crowbar-state` — first code in the crate

Two entities, deliberately separate:

* **`Entity<EntityCache>`** — flat `HashMap<String, Dto>` per kind, written only
  by the stream task. Knows nothing about the sidebar.
* **`Entity<SidebarStore>`** — the derived tree, collapse sets, active tab,
  scope registry, drag state. Rebuilt from the cache on change.

§7.1 holds: this crate owns the `Entity<T>` wrapper and the subscription graph,
nothing more. Any logic testable without `gpui` belongs one layer down, and the
split above is what makes §6 possible.

**Rebuild is not per-frame.** A stream frame marks the derived tree dirty and
notifies; the rebuild happens once per notification cycle, not once per frame
and not once per subscriber. See §9.

## 8. Interaction — the full inventory

Every row is 1:1 in behaviour. The "delta" column is visual only.

### Tier 1 · the frame

| behaviour | delta |
|---|---|
| Toggle open/collapsed, keybinding via the ported `crowbar-core::keymap` | — |
| Edge-drag resize, px min/max clamp, collapse at threshold, remembered width (§6.1) | — |
| Peek-on-hover when collapsed; `SidebarPeek` is a **wrapper, not a branch** — it renders in every state and only restyles, so hiding never rebuilds the subtree | — |
| Side left/right from `settings.sidebar_position` (already in `crowbar-core::settings::types`), as a **keyed reorder** so neither panel inherits the other's size | — |
| Tab bar: click a tab → carousel animates to that panel | — |
| Carousel: wheel/trackpad to change panel, snap to boundaries | §11.2 |
| Nav stack push/pop; header, pill and tab bar hide while a nav screen is pushed | — |
| Connection indicator (new view) | — |

### Tier 2 · read the tree

| behaviour | delta |
|---|---|
| Repos → nested workspaces, `ScrollArea`, `role=tree`/`treeitem` semantics | — |
| Expand/collapse repo sections and workspace subtrees | — |
| **Click a row → set active scope, and render it selected.** The archive's sharpest finding: `project_home_row` holds PASS 0/5 in `selected` while the shell rendered it inactive, because nothing modelled which row is active. Selection is a deliverable of this slice, not a garnish. | — |
| Project home row; project header; project switcher | §11.1 |
| Status icons: `new` · `locked` · `pr-conflicts` · `deleted` · `pr-merged` · `pr-open` · `pr-closed` | — |
| `working` spinner on rows and on the repo header (lifted to `defaultWorking`, since the default workspace is not a tree row) | — |
| Placeholder rows (`heldByPath`, no `localPath`) with their reconstructed reason | — |
| Context pill | — |
| Empty and error states; `InlineError` with retry | — |

### Tier 3 · mutate

| behaviour | delta |
|---|---|
| Inline rename of a workspace branch and of a repo; one editor open at a time, owned above the rows | — |
| Create child workspace: pending-create row, inline input, branch-name validation reserving the default branch | — |
| Placeholder actions — Retry, Detach… | — |
| Detach-holder modal | — |
| Repo import: dialog + **native folder picker** | §11.3 |
| Toasts via `sidebar_toast_overlay` — note this is the **hand-rolled** `SidebarToastItem`, which `toast.rs` does **not** cover (`QUEUE.md`) | — |
| Delete workspace | — |

### Tier 4 · drag

| behaviour | delta |
|---|---|
| Drag a workspace onto another to reparent; legality from `sidebar::drag` | — |
| Drop-target highlight on rows and repo headers | — |
| Drag ghost following the pointer | — |
| `workspace_tree_footer` trash target, revealed by height transition on drag start (new view) | — |

## 9. Performance — requirements, not aspirations

The React app's own perf findings apply directly; these are the ones this slice
can violate.

1. **No per-frame state writes during a drag.** The width lands in a pending
   slot and is committed on settle (§6.1). Same rule for the carousel offset and
   the drag ghost position.
2. **No full tree rebuild per stream frame.** A frame mutates the cache and marks
   derived state dirty; the rebuild is once per notification cycle. A repo with
   200 workspaces must not re-derive 200 nodes because one `working` flag
   flipped.
3. **Per-repo memoised subtrees.** React memoises `rootsByRepo` so a hover or
   drag re-render does not rebuild every repo's node graph. The Rust equivalent
   is keying the derived subtree by repo id and rebuilding only dirtied ones.
4. **The hidden sidebar must cost nothing.** A collapsed sidebar renders no rows.
   `QUEUE.md`'s idle-CPU findings are about exactly this class of leak.
5. **Scroll is virtualised** if a repo's workspace count makes it necessary;
   measured, not assumed.

## 10. Testing and the gates

**All four gates, zero warnings, run by me and not taken on a worker's word:**

```sh
cd native
cargo build --workspace
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
./scripts/check-invariants.sh
```

* `crowbar-core` additions carry unit tests to **≥98% lines**, ported from the
  TS suites where they exist (`__tests__/lib/store/sidebar*.test.ts`,
  `sidebar-scope`, `sidebar-sync`, `sidebar-workspace-locked`,
  `sidebar-post-delete-target`, `features/layout/stores/sidebar-nav`).
* **The transport is tested against a real socket**, not a mock: a test daemon
  fixture over a temp `UnixListener`, exercising handshake, frame delivery,
  server-side close, reconnect with backoff, and the sentinel-triggered reseed.
* **`cargo test --workspace` aborts at the first failing binary** — a green tail
  proves nothing about a crate whose binary died earlier. The run is checked for
  the full crate list, not just an exit code.
* **No timing in tests.** Block on real signals — a channel receive, a barrier —
  never a sleep.
* The frozen oracle corpus still passes unchanged. Composition does not alter
  surface identity, so a green corpus confirms the assembly was inert with
  respect to the components.

**Acceptance is the screenshot pair**, per the slice method §3.3: both apps at
one size, one `CROWBAR_HOME`, `native/scripts/capture-pair.swift`. Two capture
hazards are pre-recorded and honoured — capture only from a **settled** app
(after any nav push, carousel scroll or modal, reload first; a cluster of
`visible: false` is a contaminated capture, not a finding), and the carousel must
be on the right panel with no nav screen pushed or every anchor reports
`visible: false`.

## 11. Accepted deltas — visual only, behaviour is 1:1

1. **Menus are AppKit.** Every dropdown and right-click menu in the sidebar —
   repo header, workspace row, project switcher — is a real `NSMenu`. Its
   background, radius, border, padding, item height, font and highlight colour
   are the OS's, and keyboard nav, Escape, click-outside, submenu timing and
   screen-edge flipping become the OS's behaviour. This surface is **judged, not
   diffed**: an `NSMenu` is not in the window's view tree and carries no anchor.
   The user's decision, already formalised.
2. **Carousel physics.** CSS `scroll-snap-type: x mandatory` with momentum
   scrolling has no GPUI equivalent. The panel offset is animated to the snap
   point on tab change and on gesture end. Behaviour — which panel you land on,
   from which gesture — is 1:1; the easing curve during the gesture is not
   pixel-identical.
3. **Folder picker.** `NSOpenPanel`, not the web dialog. `crowbar-platform` owns
   it, since it is the only crate permitted `unsafe`.

Nothing else. Any further divergence found during the build is a defect to fix
or a delta to bring to the user, not a decision to make silently.

## 12. Explicitly out of scope

The three non-workspace carousel panels ship as **empty placeholders** with the
carousel mechanism fully live around them:

* `chats` → slice 3 · `files` → slice 4 · `git` → slice 5

The content pane right of the sidebar is a **blank panel**. Tabs and panes are
slices 1b and 1c. `SettingsDialog`, `TerminalHost`, `FpsOverlay` and
`FontStyleInjector` — the other `IDEShell` children — are their own slices and
are not mounted here.
