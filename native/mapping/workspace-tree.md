# `workspace-tree` (P3.61)

`web/src/components/layout/workspace-tree.tsx` →
`crates/crowbar-ui/src/components/workspace_tree.rs`,
`crates/crowbar-app/src/surfaces/workspace_tree.rs`,
`crates/crowbar-app/src/row_layout/workspace_tree.rs`.

**No live reference.** This item does not run the oracle or capture a
snapshot — per the item brief's hard constraints. Every number below is read
off the app's own compiled Tailwind (`native/MAPPING.md`'s method) or
transferred from an existing measurement, not off a live capture.

Last of `native/mapping/layout-denominator.md` §8's Cluster 8, and the
chain's own root.

## 0. What this file is, and what it is not

`workspace-tree.tsx` is `WorkspaceTree()`: the sidebar's own workspace
tree — the project-home row, a scrolling column of repo sections, each
holding its own workspace rows, and (in the real source) a trash-drop
footer this item omits (§4).

Confirmed **LIVE** by `native/mapping/layout-denominator.md` §2/§4, not by
`liveness-audit.md`.

## 1. This is the one component in the chain that IS `anchors.root`

`pending-create-row`, `workspace-tree-item` and `repo-section` are all
always composed (by a parent, or by themselves recursively), so all three
use `AnchorSink::boxed` for their own root. Nothing composes
`WorkspaceTree` — `workspace-tree.tsx`'s sole importer is `sidebar-
carousel.tsx`, which, per `native/mapping/layout-denominator.md` §1,
renders the four panels it wraps **empty**, this port included — so this
file is the chain's genuine top: [`WorkspaceTree::render`] calls
`AnchorSink::root`, `project_home_row.rs`'s own shape.

### A latent bug this composition found, and fixed

`ProjectHomeRow::render` called `AnchorSink::root` too, before this item.
Nothing had ever composed `ProjectHomeRow` inside a parent surface before
Cluster 8 landed (its sole importer was always `workspace-tree.tsx`, per
its own module docs), so the bug was never triggered. `AnchorSink::root`
clears every anchor recorded so far on entry — correct for a genuinely
top-level surface, silently wrong for one nested inside another: composing
it inside `WorkspaceTree::render` reproducibly dropped `workspace-tree`'s
own root anchor from every capture, found by this item's own `row_layout`
tests failing with the outer root missing while everything nested beneath
`project-home-row`'s own composition still appeared. Fixed in
`project_home_row.rs` (`.boxed`, not `.root`) — see that file's own module
docs for the full account.

## 2. `WorkspaceTreeFooter` is omitted, not stubbed

`workspace-tree.tsx` renders `<WorkspaceTreeFooter />` as a sibling of the
scroll area, always mounted ("always rendered so the `ScrollArea` doesn't
resize on drag start/end", the source's own comment). `native/mapping/
layout-denominator.md` §6 classifies it **Phase 5 (interaction — drag), not
Tier B**, and flags the resulting composition decision as unmade — this
item makes it: **omit**, not stub.

Its own resting picture (no drag in progress) is `max-h-0 overflow-hidden`
— zero height, by its own class list, regardless of whether the element
exists at all. Its data feed (`draggingWs`/`hoverTargetId` off
`useWorkspaceTreeDrag()`) is `workspace-tree-context.tsx`'s Phase 5
pointer-drag protocol, itself out of Tier B scope and not composed by
anything in this chain. So an omitted footer and a present-but-`max-h-0`
one paint the identical picture in every state this port's own resting
captures reach — the same `AddRepositoryModal`-shaped argument `project_
home_row.rs`'s own module docs make, reapplied to a sibling rather than a
portal. A worker who later ports `workspace-tree-footer.tsx` for its own
sake (Phase 5) is free to compose it here; this item does not block that.

## 3. `ScrollArea::render` is not called; its two ids are reused by hand

`scroll_area::ScrollArea::render` paints a **synthetic, empty** body at a
caller-fixed pixel extent — its own module docs: "unanchored on purpose…
it stands in for whatever a call site rendered." This composition needs
the opposite: the real repo-section list, sized by `flex-1` rather than a
fixed number. So this file does not call `ScrollArea::render` — the same
"does not call `X::render`" posture `project_home_row.rs`/`context_pill.rs`
already take for `Button::render`, generalized here to a primitive whose
own API has no children slot at all — and instead hand-builds the
equivalent `overflow-hidden` viewport shape, reusing `scroll_area::ID_ROOT`/
`ID_VIEWPORT` directly: the real `<ScrollArea>` in `workspace-tree.tsx`
carries exactly those two ids on its root and viewport regardless of what
it wraps, so reusing the constants keeps this port's anchor set matching
the real DOM rather than inventing a third name for the same two boxes.

## 4. The `error` early return is not modelled

`wsListData.status === 'error' && repos.length === 0` swaps the whole tree
for `<InlineError>` — `StateFlag::Error` is mandatorily unmodelled on every
surface in this port (`crowbar-app/src/surface.rs`'s own invariant), so
`WorkspaceTree` has no field for it, and the React diff does not tag that
branch's own root with `data-oracle-id="workspace-tree"` (it renders a
structurally different tree, and adding the id would misleadingly imply
this is a comparable resting state of the surface).

## 5. Anchoring

`workspace-tree.tsx` carried no `data-oracle-id` before this item. One is
added:

* `workspace-tree` — the outer `flex flex-1 flex-col overflow-hidden` div,
  this surface's own root (the main, non-error return only — see §4).

Everything else reachable under it is composed content already anchored by
its own port: `project-home-row`'s own five ids (unconditional),
`scroll-area-root`/`scroll-area-viewport` (§3), and, per repo,
`repo-section`'s own family (excluded from this surface's own declared
scope — see below).

### The scope-entry decision, argued in full

**`web/src/lib/oracle/extract.ts` declares `workspace-tree`: the outer
scaffold only** — the root, `project-home-row`'s own five ids (already
declared under their own names, reused rather than repeated here —
`command`'s own `scroll-area-*` reuse, one call further), and the
hand-built `scroll-area-root`/`scroll-area-viewport` pair. `repo-section`
is excluded for `select-item`'s reason one level up: the repo count is a
property of the workspace list, not of this surface. Whatever a
`repo-section` capture nests (in turn excluding its own `workspace-tree-
item`/`pending-create-row` families) is verified through that surface's
own root, never through this one — `resizable`'s own precedent for a
container whose repeated content is somebody else's surface.

## 6. Declarations

`WorkspaceTree` paints no text of its own — every text run reachable under
its root belongs to a composed child (`project-home-row`'s label,
`repo-section`'s, `workspace-tree-item`'s), each already declared on its
own surface. No `CONTENT_SIZED`/`LINE_SIZED` array on this file.

## 7. The state axis

| flag | here |
|---|---|
| `loading`, `error`, `hover`, `focus`, `selected`, `empty` | **unmodelled.** This surface is a container, not a row — it has no selection/hover/focus picture of its own, and its own `empty` (zero repos) would be a real, drivable state (`--repos 0` already reaches it structurally) but is left undeclared as a §8.3 flag: nothing in `native/mapping/layout-denominator.md` flags a live "zero repos" reference to validate a field against, and inventing one without it is the trap `ANCHORS.md` §6 warns against. `--repos <n>` still lets a future worker drive the count directly. |

`Params::no_state_axis()` returns `true` — none of the six §8.3 flags has
a rule on this surface, `workspace_branch_icon.rs`'s own precedent for the
declaration.

## 8. `row_layout` coverage

* the default cell (one repo) carries the root, the full `project-home-row`
  family, the hand-built scroll-area pair, and one nested `repo-section`
* `--repos 3` nests three `repo-section` roots
* `--repos 0` still carries the scaffold — no `repo-section` at all,
  nothing else missing

No label-height assertion in this file's own `row_layout` module — see §6:
this surface paints no text run of its own for such an assertion to name.

## 9. Reachability

`sidebar-carousel.tsx` → `ide-shell.tsx` → `routes/_shell.tsx`. Always
mounted above the repo tree, per `native/mapping/layout-denominator.md`'s
own table.
