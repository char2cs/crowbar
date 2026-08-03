# `sidebar-skeleton` (P3.55)

`web/src/components/layout/sidebar-skeleton.tsx` →
`crates/crowbar-ui/src/components/sidebar_skeleton.rs`,
`crates/crowbar-app/src/surfaces/sidebar_skeleton.rs`,
`crates/crowbar-app/src/row_layout/sidebar_skeleton.rs`.

> Cluster 3, "standalone sidebar chrome" (`native/mapping/layout-denominator.md`
> §8). Composes eighteen instances of `web/src/components/ui/skeleton.tsx`'s
> `<Skeleton>` — already ported as `crate::components::skeleton` — across five
> distinct box shapes, two more than that module's own `CallSite` enum names.

**Reference: none, and provably so rather than merely unobserved.**

## 0. The gap this item closes

`sidebar_skeleton.rs` (the `crowbar-ui` port) predates this item and already
carried the composition's arithmetic and its own unit tests, but no
`--surface` existed to drive it — the exact same shape of gap
`sidebar-project-header.md` §0 describes. This item adds
`crowbar-app/src/surfaces/sidebar_skeleton.rs` and
`crowbar-app/src/row_layout/sidebar_skeleton.rs`.

## 1. Why there is no reference: proven never to mount, not merely unobserved

`skeleton.rs`'s own module docs already establish the stronger form of this
finding for the *primitive*: `<Skeleton>`'s only call site anywhere in
`web/src` is `<SidebarSkeleton>`'s own rows, and `<SidebarSkeleton>`'s own
only call site is a `<Suspense fallback={<SidebarSkeleton />}>` wrapping
`FileExplorerTree` — a **static import** with no suspending source in its
subtree (`grep` finds no `React.lazy`, `useSuspenseQuery` or `use()` under
it). A `<Suspense>` boundary shows its fallback only while a descendant
suspends, so the fallback never mounts: a live count of
`[data-slot="skeleton"]` was **0** in every state, including immediately
after a full reload. That finding applies with exactly the same force to
this composition, because `<SidebarSkeleton>` **is** that fallback — there is
no other call site for it to be captured from, ever, on any build. No
reference was captured, attempted through a synthetic trigger, or fabricated;
there is no `/tmp/p3-ref-sidebar-skeleton.json`.

`web/src/__tests__/lib/oracle/extract.test.ts` already carries the other half
of the same fact: `expect(oracleSurfaceScope('sidebar-skeleton')).toBeNull()`
— no scope declaration exists or is needed, because every one of this
composition's eighteen bars plus its own divider is authored under this
file's own `sidebar-skeleton-*` namespace, with nothing foreign nested inside
it (contrast `sidebar-project-header`, which needs a scope entry precisely
because it nests a foreign, separately-registered anchor).

## 2. The composition has no parameters at all

`sidebar-skeleton.tsx` hardcodes `[1, 2].map(...)` three times — two chat
rows, two repo groups, two workspace rows per group — with no prop of any
kind. `SidebarSkeleton` is a unit struct on the `crowbar-ui` side, and this
surface's own `Params` is one too: there is no option to drive, because there
is no seam in the React original to drive it through.

## 3. Five shapes, and why this port does not touch `skeleton::CallSite`

| Row | Bars | Shape |
|---|---|---|
| chat row (×2) | icon, title, meta | `h-5 w-5 rounded-md` · `h-3 flex-1 rounded` · `h-3 w-8 rounded` |
| repo header row (×2) | icon, name | `h-5 w-5 rounded-md` · **`h-3 w-24 rounded`** |
| workspace row (×4) | title, meta | `h-3 flex-1 rounded` · **`h-3 w-12 rounded`** |

The two bold shapes — a fixed **96px** repo name and a fixed **48px**
workspace meta bar — are not in `skeleton::CallSite`'s four arms
(`None`/`Icon`/`Title`/`Meta`, where `Meta` is the chat row's own **32px**
bar). `Skeleton` carries no `id` field of its own — every instance anchors
under the fixed `skeleton::ID_SKELETON`, right for one placeholder measured
on its own and wrong for eighteen under one root (the same collision
`sidebar_project_header.rs`'s module docs record for `Button`). This file
instead reuses `skeleton.rs`'s own public values
(`skeleton::RADIUS_CALL_SITE`, `theme.color_muted`/`theme.radius_md`) to
paint the two shapes `CallSite` already covers, and adds its own two
constants — `REPO_NAME_WIDTH`, `WORKSPACE_META_WIDTH` — for the two it does
not, each with its own authored anchor id.

## 4. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `py-1` (outer column) | 4px | `OUTER_PADDING_Y` |
| `px-2` (outer column) | 8px | `OUTER_PADDING_X` |
| `space-y-0.5` (between the five top-level children) | 2px | `OUTER_GAP`, reproduced as a flex `gap` rather than the `margin-top`-on-siblings CSS uses — the two produce the same picture for a column with no `flex-wrap` |
| `space-y-0.5` (inside a repo group) | 2px | `GROUP_GAP` |
| `h-9` (every row) | 36px | `ROW_HEIGHT` |
| `gap-2` (a row's own bars) | 8px | `ROW_GAP` |
| `mx-1.5` (a row's own horizontal margin) | 6px | `ROW_MARGIN_X` |
| `px-2` (a row's own internal padding) | 8px | `ROW_PADDING_X` |
| `pl-6` (workspace row) | 24px, **replaces** `px-2`'s left half rather than adding to it (`tailwind-merge`) | `WORKSPACE_ROW_PADDING_LEFT` |
| `h-5 w-5` (icon bar) | 20 × 20 | `ICON_EXTENT` |
| `h-3` (every text-shaped bar) | 12px | `TEXT_BAR_HEIGHT` |
| `w-8` (chat meta) | 32px | `CHAT_META_WIDTH` |
| `w-24` (repo name) | 96px — **not** in `CallSite` | `REPO_NAME_WIDTH` |
| `w-12` (workspace meta) | 48px — **not** in `CallSite` | `WORKSPACE_META_WIDTH` |
| `my-1 mx-3 h-px` (divider) | 4px / 12px / 1px | `DIVIDER_MARGIN_Y` / `_X` / `DIVIDER_HEIGHT` |

## 5. Declarations

`CONTENT_SIZED = []` — every bar pins both axes or takes `flex-1`; none is a
content width, the same finding `skeleton.rs` already makes about the
primitive. `LINE_SIZED = []` — no anchor here paints text.

## 6. The state axis

Every one of the six §8.3 flags is unmodelled: `sidebar-skeleton.tsx` renders
no interaction rule of any kind, and this is not a row, so `Empty` does not
apply either — every one of the eighteen bars is unconditional, hardcoded by
the two `[1, 2].map` calls the component takes no prop to bypass. Checked
exhaustively, the same standard `fps-overlay`'s and
`workspace-branch-icon`'s own module docs set: `export function
SidebarSkeleton()` takes no props at all. `Params::no_state_axis()` returns
`true`.

## 7. `row_layout` coverage, and the arithmetic it confirms

The whole column's own height at the default 320px width is **321px** —
`py-1` (4 + 4), five top-level children (two 36px chat rows, a 9px divider
counting its own `my-1` margin, two 112px repo groups) and four
`space-y-0.5` (2px) gaps — read off a real taffy layout
(`the_columns_own_height_is_three_hundred_and_twenty_one_pixels`) rather than
only asserted from the hand arithmetic that predicts it. Also covered:

* all twenty anchors present exactly once (root + eighteen bars + the
  divider), matching this doc's own count
* the two chat rows are offset by a row height plus the outer gap (38px)
* each repo group is a header row plus two `pl-6`-indented workspace rows,
  each offset by a row height plus the group's own gap (38px); the
  workspace rows' own content starts 16px further right than the header
  row's (`pl-6`'s 24px replacing `px-2`'s 8px left half, not adding to it)
* the two repo groups are offset by one group's own total height plus the
  outer gap (114px)
* the divider sits strictly between the chat rows and the first repo group,
  is exactly 1px tall, and paints no text
* the two bars `skeleton::CallSite` does not carry (repo name, workspace
  meta) stay fixed as the column widens; the two `flex-1` title bars grow
  with it, pixel for pixel
* every icon bar is 20 × 20 with `theme.radius_md`

## 8. Reachability

`sidebar-carousel.tsx` is the one importer
(`native/mapping/layout-denominator.md` §2), as the `<Suspense>` fallback §1
describes — mounted in source, never in a running app.
