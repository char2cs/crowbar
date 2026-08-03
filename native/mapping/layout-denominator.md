# The `components/layout` denominator (P3.48)

Companion to QUEUE.md's "The real Tier B denominator" (the `components/ui`
measurement), applied to the other half of §16 Phase 3 Tier B: *"the 46
`components/ui` primitives **and 36 `components/layout` files**."*

**Status: IN PROGRESS.** This is a checkpoint commit — rows below are final
where marked, `TODO` where not yet measured. Survey only; nothing under
`native/crates/` was touched, the oracle was not run, `crowbar-app` was not
launched.

## 0. The 36 vs 29 reconciliation — the spec's count and disk agree once you count `.ts` too

§3.2 and §5.1 both say **36** `components/layout` files. `ls
web/src/components/layout/*.tsx` finds **29**. This is not drift, and it is
not the spec being stale (as QUEUE.md's 2026-08-03 entry speculated) — it is
this survey counting the wrong extension:

```
$ ls web/src/components/layout/*.ts | wc -l
7
```

`context-pill-model.ts` · `format-change-count.ts` · `use-sidebar-panel.ts` ·
`workspace-row-base.ts` · `workspace-switcher-model.ts` ·
`workspace-tree-actions.ts` · `workspace-tree-utils.ts`.

**29 + 7 = 36.** The spec's denominator was always "every file in the
directory", `.tsx` and `.ts` alike — it just happens that `components/ui`'s
own 72/46 count is `.tsx`-only, which is why the brief (reasonably) framed
this survey the same way. Per the brief's own scope ("`web/src/components/
layout/*.tsx`. The 29 files.") this report classifies the 29 `.tsx` files
only. The 7 `.ts` files are pure logic/helpers (a model derivation, row-class
constants, tree utilities, a hook) with no JSX and nothing for the oracle to
anchor — they are Tier A-shaped (ported as plain logic, gated by unit tests,
not by the oracle) regardless of this survey's verdict on their `.tsx`
callers, so excluding them from a "does the oracle anchor this" table is
correct, not an oversight. Flagged here so the 36/29 gap is never again read
as the tree having moved.

## 1. The control: `sidebar-carousel.tsx` — confirmed ported

`native/crates/crowbar-app/src/surfaces/sidebar_carousel.rs` (506 lines) +
`native/crates/crowbar-ui/src/components/sidebar_carousel.rs` (450 lines).
Verified live in QUEUE.md ("`sidebar-carousel` VERIFIED by my own side-by-side
run — 2 cells, 0 deltas", and again in the Phase 2 close: "2 cells, 0 deltas
over 5 anchors, including the tangent `visible` case"). Confirmed here by
reading both Rust files, not just trusting the log: the port anchors the
scrollport + four panel boxes and their `selected`/`visible` states, and
**explicitly renders the four panels' contents empty** — `WorkspaceTree`,
`AgentChatsPanel`, `FileExplorerTree`, `GitPanel` are out of this port's
reach, and so, by the same logic, are `NavStack` and `SidebarSkeleton`, which
`sidebar-carousel.tsx` also wraps/references but which the Rust port does not
render at all. **"Ported" here means the outer scroll-track geometry only —
it does not transitively cover anything it wraps.** This matters for several
rows below.

## 2. Per-file table

Legend: **Importers** lists non-test importers only, resolved through both
`@/components/layout/X` and relative `./X` spellings (script:
`find_importers.py`, cross-checked by hand for every row below — see §3 for
the method). A file with only `ide-shell.tsx`/another layout file as importer
is still counted as used; "dead" is reserved for files with **zero**
resolved importers of either spelling, test or otherwise.

| File | Lines | Non-test importers | Store(s) | `useEffect` | Verdict |
|---|---|---|---|---|---|
| `sidebar-carousel.tsx` | 173 | `ide-shell.tsx` | `useFileSystemStore`, `useFileTreeStore`, `useSidebarStore` | 2 | **already ported** (control) |
| `ide-shell.tsx` | 281 | `routes/_shell.tsx` | `useProjectDataStore`, `useProjectStore`, `useSettingsStore`, `useSidebarNavStore`, `useSidebarStore` | 3 | **Phase 4/6 — app shell**, not a Tier B target in its own right (see §4) |
| `nav-stack.tsx` | 74 | `sidebar-carousel.tsx` | `useSettingsStore`, `useSidebarNavStore` | 0 | **Tier B target** — judgment call, see §4 |
| `fps-overlay.tsx` | 93 | `ide-shell.tsx` | `useSettingsStore` | 1 | **Tier B target** — not build-flag dev-only, see §4 |
| `connection-indicator.tsx` | 68 | `ide-shell.tsx` | `useConnectionStore` | 1 | **Phase 4 (state)** — renders `null`, pure store watcher |
| `detach-holder-modal.tsx` | 68 | `ide-shell.tsx` | `useDetachModalStore` | 0 | **Tier B target** |
| `drag-ghost.tsx` | 50 | `agent-chats-panel.tsx`, `use-agent-chat-list-drag.ts`, `workspace-tree-context.tsx` | none | 0 | **Phase 5 (interaction — drag)** |
| `placeholder-row-actions.tsx` | 59 | `workspace-tree-item.tsx` | `useDetachModalStore` | 0 | **Tier B target** |
| `placeholder-toast-watcher.tsx` | 39 | `ide-shell.tsx` | `useDetachModalStore`, `useSidebarStore` | 1 | **Phase 4 (state)** — renders `null`, pure store watcher |
| `project-home-row.tsx` | 119 | `workspace-tree.tsx` | `useHomeWorkspaceStore`, `useProjectDataStore`, `useProjectStore`, `useSidebarNavStore` | 0 | **Tier B target** |
| `project-switcher-panel.tsx` | 83 | `project-home-row.tsx` | `useProjectDataStore`, `useProjectStore`, `useSidebarNavStore` | 0 | **Tier B target** |
| `repo-avatar.tsx` | 90 | `new-tab-view.tsx` (features/panes), `context-pill.tsx`, `repo-icon-popover.tsx`, `workspace-switcher.tsx`, + 2 `.ts` helpers | none | 0 | **Tier B target** |
| `repo-icon-popover.tsx` | 311 | `repo-section.tsx` | none | 0 | **Tier B target** |
| `repo-import-dialog.tsx` | 208 | `repo-section.tsx` | none | 1 | **Tier B target** |
| `repo-section.tsx` | 291 | `workspace-tree.tsx` | `useSidebarStore` (event-handler only) | 0 | **Tier B target** |
| `sidebar-peek.tsx` | 151 | `ide-shell.tsx` | none | 1 | **Tier B target** — judgment call, see §4 |
| `sidebar-project-header.tsx` | 104 | `ide-shell.tsx` | `useSettingsStore` | 0 | **Tier B target** |
| `sidebar-skeleton.tsx` | 32 | `sidebar-carousel.tsx` | none | 0 | **Tier B target** |
| `sidebar-tab-bar.tsx` | 69 | `ide-shell.tsx` | `useSidebarStore` | 1 | **Tier B target** |
| `sidebar-toast-overlay.tsx` | 183 | `ide-shell.tsx` | none (uses `toastManager` directly) | 0 | **Tier B target — NOT already covered**, see §5 (important correction) |
| `context-pill.tsx` | 111 | `ide-shell.tsx` | `useHomeWorkspaceStore`, `useProjectDataStore`, `useProjectStore`, `useSidebarStore` | 0 | **Tier B target** |
| `workspace-branch-icon.tsx` | 72 | `context-pill.tsx`, `pending-create-row.tsx`, `project-home-row.tsx`, `repo-icon-popover.tsx`, `workspace-switcher.tsx`, `workspace-tree-item.tsx` | none | 0 | **Tier B target — NOT already covered**, see §5 |
| `workspace-inline-input.tsx` | 98 | `agent-chat-row.tsx` (features/agent), `repo-section.tsx`, `workspace-tree-item.tsx` | none | 1 | **Tier B target** |
| `workspace-switcher.tsx` | 217 | `context-pill.tsx` | `useProjectDataStore`, `useProjectStore`, `useSidebarStore` | 1 | **Tier B target** |
| `pending-create-row.tsx` | 54 | TODO | TODO | 0 | TODO |
| `workspace-tree-context.tsx` | 464 | `pending-create-row.tsx`, `repo-section.tsx`, `workspace-tree.tsx`, `workspace-tree-footer.tsx`, `workspace-tree-item.tsx`, `workspace-tree-actions.ts` | `useSidebarStore` | 1 | TODO |
| `workspace-tree-footer.tsx` | 44 | `workspace-tree.tsx` | none | 0 | TODO |
| `workspace-tree-item.tsx` | 318 | `repo-section.tsx` | `useSidebarStore` | 0 | TODO |
| `workspace-tree.tsx` | 110 | `sidebar-carousel.tsx` | `useSidebarStore`, `useWorkspaceListStore` | 0 | TODO |

No file among the 29 has **zero** resolved importers by either spelling —
every one is reached from somewhere in `web/src`, so **no "dead" verdicts on
this directory** (contrast `components/ui`, which had a live "wrongly called
dead" trap the brief warned about — see §3 for why that trap did not
reproduce here).

## 3. Method — both import spellings, resolved programmatically

Same requirement as the `ui/` denominator: "a file is only 'unused' once you
have searched every import spelling." A regex/grep-only pass over `@/
components/layout/X` alone would miss every relative `./X` import — which,
inside `components/layout/` itself, is the *majority* of intra-directory
references (e.g. `sidebar-carousel.tsx` imports `./nav-stack`, `./workspace-
tree`, `./sidebar-skeleton` — none of which is the `@/…` form).

Built `find_importers.py`: walks every `.ts`/`.tsx` under `web/src`
(excluding `node_modules`), regex-extracts every `import … from '…'` /
dynamic-`import('…')` specifier, and for each of the 29 targets resolves a
hit two ways — literal-prefix match against `@/components/layout/<name>`, and
`os.path.normpath` resolution of every relative specifier against the
importing file's own directory, compared against the target's real path.
This is the same two-spelling requirement the `ui/` denominator's own
retraction note describes ("my first pass used only [the absolute] form and
wrongly called two of these dead").

**That trap did not reproduce here**, and it's worth saying why rather than
just reporting a clean result: `components/ui/inline-combobox.tsx` and
`table-icons.tsx` were missed because their *sole* importer was a Plate node
reached only by a relative import from deep inside `components/ui/block-
plate/…`, invisible to an absolute-only grep. Nothing in `components/layout/`
is Plate-adjacent in that way — the relative imports here are all siblings
within the same flat directory (`./nav-stack`, `./workspace-tree`, etc.), so
an absolute-only pass would have under-counted by omission of *volume*, not
by omission of a *hidden* importer the way the `ui/` case did. Still resolved
both ways per the brief's instruction; the two-spelling method is what makes
that a measured fact rather than an assumption.

## 4. The two contentious calls, argued in full

### `ide-shell.tsx` — Phase 4/6, not a Tier B target

Read in full (281 lines). It is the router-scope resolver (`recordWorkspace-
ScopeFromPath`, synchronously during render, with an explicit comment about
*why* it must run before `WorkspaceHost`'s own render), the home-workspace
resolution cache, the active-project sync effect (`BUG-003`), the sidebar-
panel-collapse-from-store effect, the keyed-panel-identity fix for flipping
sidebar sides (a whole paragraph of comment explaining a React reconciliation
bug), and the mount point for `WorkspaceHost`, `SettingsDialog`,
`TerminalHost`, `FontStyleInjector`, plus five of this directory's own
components. Its own JSX is almost entirely composition — `<ResizablePanel-
Group>` wrapping two `<ResizablePanel>`s — with the actual visual surface
delegated to its children.

This matches the brief's own framing of the contentious question exactly:
the `ui/` work already treats `ide-shell.tsx` as a **call site** for
`resizable` (i.e., evidence that `resizable.tsx`'s anchors were captured by
driving it inside this shell), not as a component with its own anchor set.
Its substance — route-derived state, effect-driven panel imperative calls,
cross-tree mount lifecycle — is exactly Phase 4's definition ("store wiring /
`useEffect` orchestration, not layout") and Phase 6's ("parity sign-off, then
migrate" — the shell is what phase 6 replaces wholesale). **Verdict: Phase
4/6 (app shell), not counted toward the Tier B target total.** The
`resizable` panel geometry it hosts is already covered by `resizable`'s own
port; nothing here is left unanchored by that exclusion.

### `fps-overlay.tsx` — Tier B target, and "dev-only" is the wrong frame

Gated by `useSettingsStore((s) => s.settings.showFpsOverlay)` — grepped the
setting itself: it is a real field in `features/settings/types/settings.ts`,
defaults to `false` in `default-settings.ts`, and is exposed as a toggle in
`features/settings/components/tabs/developer-settings.tsx` (a "Developer"
settings tab, not a hidden flag). **There is no `NODE_ENV`/`import.meta.env`
check anywhere in this file or its gate.** It ships in every build, including
production, and any user who opens Settings → Developer and flips the switch
sees it — same posture as `showFpsOverlay`'s sibling toggles in that tab.

So the brief's framing ("if it is dev-only, say what makes it dev-only —
build flag, settings toggle, `NODE_ENV` check") resolves to: **a settings
toggle**, which the brief explicitly listed as one of three possible
mechanisms, not as evidence against dev-only-ness. Whether "gated behind a
Developer settings toggle, shipped in every build" counts as "dev-only" is a
naming question, not a scope question — either way it is a real, reachable,
presentational surface (fixed-position badge, three-way color threshold on
fps, tabular-nums drop counter) with nothing about it that is Plate, state-
orchestration, or interaction-substance. **Verdict: Tier B target.** The
`rAF`-driven number itself is real-time instrumentation the oracle cannot
reproduce identically, but the component's *anchors* (badge position, text
runs, color-token switch) are exactly as drivable as `badge.tsx`'s own state
matrix — pass the three colour/number states as fixture input the way every
state-driven `ui/` primitive already does.

### `nav-stack.tsx` and `sidebar-peek.tsx` — Tier B by the `sidebar-carousel` precedent, flagged rather than asserted

Both are structurally the same shape as `sidebar-carousel.tsx` itself: a
store-driven container whose CSS-transition-based state (push/pop screen
stack for `nav-stack`; docked/closed/peeking for `sidebar-peek`) produces
concrete, anchorable end-state geometry, with the *animation itself* — not
the rest states — being the part gpui cannot express. `sidebar-carousel.rs`'s
own module docs already settled this exact tension for the carousel: "What
the port reproduces is not the snap engine but the snapped position," and
`ANCHORS.md` §6 puts "a snapshot is one instant" beyond the oracle's
contract. Applying the same reasoning:

- `nav-stack.tsx`: substance is `useSidebarNavStore`'s push/pop stack,
  rendered as `-translate-x-1/4 opacity-0` (hidden) vs `translate-x-0
  opacity-100` (top) vs off-stage (`translate-x-full`) panels, each state
  reachable by driving the store directly (`push`/`pop`), exactly how
  `sidebar-carousel` is driven by calling `setActiveTab`. Its header (back
  button + title) is plain presentational geometry with a comment tying its
  height/padding directly to `SidebarProjectHeader`'s.
- `sidebar-peek.tsx`: substance is a `document`-level `pointermove` listener
  computing a `hovered` boolean from raw cursor coordinates against an edge-
  trigger band — genuinely more "interaction" in flavor than `nav-stack`,
  since the *trigger* is continuous pointer tracking, not a discrete store
  action. But the boolean it produces is a single `hovered` state that
  drives three concrete geometries (`data-state="docked"/"closed"/
  "peeking"`), drivable directly without simulating real mouse movement —
  same shape as sidebar-carousel's `selected` cell.

Both are recorded as **Tier B target**, but flagged, per the brief's request,
rather than silently asserted: the alternative reading (interaction
substance → Phase 5) is defensible for `sidebar-peek.tsx` in particular, and
a future worker should not treat this call as settled the way `ide-shell`'s
is. If reclassified Phase 5, the file's rest-state geometry would still need
porting eventually as part of Phase 5's "extend the oracle with interaction
record/replay" — the question is only which phase's gate it goes through
first.

## 5. `sidebar-toast-overlay.tsx` and `workspace-branch-icon.tsx` — checked against "already covered" and found NOT covered

The verdict table includes "already covered — an existing registered surface
already measures it" as an option, and two files in this directory have
surface-shaped names elsewhere in the tree that make that verdict tempting.
Checked both against the actual Rust source rather than the surface list's
naming.

**`sidebar-toast-overlay.tsx` vs `toast.rs`.** `native/mapping/toast.md`
already documents, in its own §2 ("Reachability: zero, and it is provable
rather than merely unobserved"), that `toast.rs` was built against `ui/
toast.tsx`'s `AnchoredToasts` export — which **no code path in this
application ever calls** (`grep -rn anchoredToastManager web/src` finds only
the declaration, its own provider, and a re-export; zero `.add(` call
sites). The doc's own words: *"Every real toast the running app shows a user
goes through the **other** manager … rendered by a **third, unrelated
file** — `components/layout/sidebar-toast-overlay.tsx`'s own hand-rolled
`SidebarToastItem`."* The doc even tabulates concrete shape differences
between `toast.rs`'s reference (the dead `tooltipStyle` arm) and this file's
real rendering — different width behavior, different title styling, no
vendor widget in either's path. **`sidebar-toast-overlay.tsx` is the live,
reachable toast surface and it has never been ported; `toast.rs` ported an
unreachable sibling.** This is exactly the class of gap the brief asked me to
surface rather than wave through on the strength of a same-named surface
file existing. **Verdict: Tier B target, not already covered.**

**`workspace-branch-icon.tsx` vs `flicker_spinner.rs` / `git_status_row.rs`.**
`grep -rn "workspace-branch-icon\|WorkspaceBranchIcon"
native/crates native/mapping` finds exactly three hits, all in `flicker-
spinner.md`/`flicker_spinner.rs`, and all of them are **measurement
provenance for `FlickerSpinner`'s own size** ("`workspace-branch-icon.tsx`'s
`size-3.5`, inside `WorkspaceAgentSpinner`'s `size-4` box") — i.e.
`workspace-branch-icon.tsx` is cited as a *caller* whose usage justified one
of `flicker-spinner`'s state fixtures, not as a component that was itself
ported. `git_status_row.rs` is unrelated: it is the git-status **panel**
row (`features/git/`), a different tree entirely, sharing no code with the
sidebar's per-status branch icon. `WorkspaceBranchIcon`'s own eight-way
status switch (`locked`/`new`/`pr-conflicts`/`deleted`/`pr-open`/`pr-closed`/
`pr-merged`, plus `working`/`isPlaceholder` overrides) has no Rust
counterpart anywhere in the tree. **Verdict: Tier B target, not already
covered.**

## 6. Headline numbers (partial — 24 of 29 classified)

TODO — final count pending the remaining 5 rows (`pending-create-row`,
`workspace-tree-context`, `workspace-tree-footer`, `workspace-tree-item`,
`workspace-tree`).

Running tally through row 24:

| Verdict | Count so far |
|---|---|
| already ported (control) | 1 (`sidebar-carousel`) |
| Tier B target | 20 |
| Phase 4 (state) | 2 (`connection-indicator`, `placeholder-toast-watcher`) |
| Phase 5 (interaction) | 1 (`drag-ghost`) |
| Phase 4/6 (app shell) | 1 (`ide-shell`) |
| dead | 0 |
| Plate-only | 0 |
| out of scope, other | 0 |

## 7. Wave split — TODO, pending final Tier B list
