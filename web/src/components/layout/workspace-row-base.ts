// spec §4.1: switching spaces is a `wheel` gesture and dragging a row onto a
// pane is a `pointer` drag, so a row may not disable either scroll axis —
// `touch-action: none` would starve the space-switch wheel gesture the rail
// now scrolls both ways for. Explicit `pan-x pan-y` rather than the `auto`
// default so the row states its own contract instead of inheriting whatever
// the browser assumes.
// `contain-layout` (CSS `contain: layout`) scopes each row's own reflow to
// itself — live-measured (rAF-delta sampling + a real continuous swipe, not
// a discrete jump) as the actual fix for a real regression: ROW_SUB_ACTION_HOVER's
// `hidden`→`inline-flex` toggle (see its own doc comment — deliberately kept,
// a prior `invisible`-based attempt is the documented reason NOT to swap it
// for an opacity toggle) was already measured as affordable for ONE row, but
// a fast flick down the list can toggle MANY rows within a single frame, and
// without containment the browser's own layout invalidation isn't scoped to
// just the touched rows — 29/199 frames over 16.7ms, max 65ms. Safe here
// specifically because every row has a fixed height (`h-9`) and no
// non-portaled `position: absolute` descendant that would need a farther
// containing block. With it: 1/199 frames over 16.7ms, max 18ms, same
// numbers as reserving the buttons' space outright — but without that
// approach's already-documented cost (permanently truncating the label).
export const ROW_BASE =
  'flex cursor-pointer select-none items-center gap-1.5 rounded-lg border contain-layout ' +
  'h-9 px-1.5 mx-1.5 my-0.5 text-[13px] font-medium outline-none [touch-action:pan-x_pan-y] ' +
  'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background'

// Shared selected/idle row variants so every sidebar row (workspaces, the repo
// header) renders an identical active state — a raised, inset-lit surface when
// selected and a flat hover-accent when not. Keep both in sync here, never inline.
//
// Deliberately the OTHER theme's surface, not this one's: `bg-background`/
// `text-foreground` would just track the ambient theme like every other
// token on the row, and the point of this one is that it doesn't — the
// active row is meant to visibly break the theme it sits in (light sidebar,
// dark active row; dark sidebar, light active row), so the thing that's
// selected pops rather than blending in. `--background-inverse`/
// `--foreground-inverse` (theme.css) are the other theme's own values,
// literally, not derived from this theme's tokens. The border matches the
// (inverted) background, same as it always matched `--background` before —
// there is no visible border, it's there so the row's box-sizing agrees with
// its own fill at the 1px edge.
// The highlight itself (`--row-active-highlight`, theme.css) is production's
// (develop) OWN `--elevated-highlight` recipe, unchanged in every respect
// (plain `0_1px`, `color-mix` at 16%, zero blur) except which foreground it
// mixes — `-inverse`, to pair with this row's deliberately inverted
// background. Production's ROW_ACTIVE never inverts, so it just reuses
// `--elevated-highlight` verbatim; see theme.css's doc on this token for why
// every other version tried here (a separate before-overlay, a colored
// accent, hand-tuned alpha/blur/offset) was solving a problem production's
// own recipe never had.
//
// `outline` instead of `border` for the edge: a real `border` utility adds
// its own width to the box (border-box eats it out of content on a fixed-
// height row, fine — but a caller with no explicit height of its own, e.g.
// Recents' multi-chat shell, has that width added STRAIGHT to its auto
// height instead, measurably taller than every plain row even though the
// color always matches the fill and the edge is never visible either way).
// `outline` paints the identical 1px edge without ever participating in
// layout, on any caller, explicit height or not.
export const ROW_ACTIVE =
  'outline outline-1 outline-background-inverse bg-background-inverse text-foreground-inverse shadow-xs shadow-black/10 ' +
  'not-disabled:inset-shadow-[0_1px_var(--row-active-highlight)] ' +
  'active:inset-shadow-[0_1px_--theme(--color-black/8%)] active:shadow-none'

// Plain flat tokens, deliberately with NO CossUI top-highlight — that glossy
// treatment (ROW_ACTIVE above, and recents-band.tsx's own group-hover
// treatment) is reserved for a row that IS the showing/selected surface, or a
// grouped Recents SET being hovered as one unit. An ordinary tree row and its
// own plain hover, and a merely has-a-view tree row, are neither of those —
// a corrected misstep (an earlier pass here put the CossUI shadow on both,
// "on all the sidebar rows," which was too broad — user correction: the tree
// never gets it, and hover, on its own, never gets it either. Only Recents'
// ACTIVE row/group does.
// Hover uses the same `bg-sidebar-element-idle` wash an already-open row sits
// on at rest (ROW_HAS_VIEW_IDLE, below) — not `bg-accent`, a second muted
// token that happened to differ from it (a flat opaque color in dark mode
// against this one's translucent foreground wash), which made a plain row's
// hover and an already-open row's resting ground read as two different
// mechanisms for the same "something is here" signal.
export const ROW_INACTIVE = 'border-transparent text-foreground hover:bg-sidebar-element-idle'

// A row that HAS a view (open, somewhere) but is not the one on screen right
// now — the tree's own version of the middle state Recents itself marks with
// `hasView`'s greyed label (recents-band.tsx draws no SET shell ground for
// this state either — that file's own doc on why a dormant/parked SET
// matches a solo one, with no fill of its own). The tree never draws
// ROW_ACTIVE (sidebar-row.tsx's own doc: that surface "moved to Recents"), so
// there's no risk of this idle ground reading louder than an active row.
// Replaces a muted TEXT COLOR the tree used to fall back to for the same
// signal (`hasView && text-muted-foreground`) — the row is open, not
// disabled, so it should look present, not grey.
export const ROW_HAS_VIEW_IDLE = 'bg-sidebar-element-idle'

// The nest signal: the row a drop would land INSIDE fills.
//
// Two signals, never both. A hairline (drop-indicator.tsx) says "between these
// rows"; this says "inside this one". Drawing both at once leaves the user to
// guess which of two genuinely different moves — a reorder and a re-parent — is
// about to happen, and re-parenting a fork is the expensive one to guess wrong.
//
// A fill rather than the ring the row used to take: a ring reads as focus,
// which is a state the row can already be in for unrelated reasons, and it
// traces the same 1px outline whether the drop lands inside the row or beside
// it.
//
// Its own tokens rather than a --primary tint: --primary is the app's most
// emphatic surface (the active pane ring, the ghost's count badge), and a drop
// mark has to read over a row already wearing its hover or active treatment
// without shouting over either. See theme.css for the two values, which the
// hairline shares so both marks are visibly the same gesture.
export const ROW_NEST_TARGET = 'border-sidebar-drop-nest-edge bg-sidebar-drop-nest text-foreground'

// The disclosure chevron, drawn once. Every expandable row in the sidebar —
// project, repo, folder, workspace — closes with the same mark in the same
// trailing slot, rotated 90° when open. It used to be copy-pasted at four call
// sites, and the project row had drifted into a different gesture entirely (a
// LEADING glyph that swapped to a chevron on hover), which made the one control
// the tree repeats most often the one control it drew two ways. Drawn inline
// off this one path — the unified SidebarRow (sidebar-row.tsx) is the sole
// caller now, so there is only one call site left to keep in sync.
export const DISCLOSURE_GLYPH_PATH = 'M6 3l5 5-5 5'

// One tree level, in px. Every indented wrapper steps by this, so a row hoisted
// under a folded parent lands exactly one step in.
export const ROW_INDENT_STEP = 14

// A row's indent animates, because it MOVES: a row kept through a collapse is
// re-drawn one step under the parent that is holding it, whatever depth it
// really lives at. `margin-inline-start` rather than padding, so the box that
// moves is the row's own and not the column around it.
//
// `motion-safe:` rather than a media query of our own — the whole redesign's
// motion is opt-out, and this is the variant that already means that.
export const ROW_INDENT_TRANSITION = 'motion-safe:[transition:margin-inline-start_0.1s_ease-in-out]'

// Fork's naming input (sidebar-row.tsx's `PendingSidebarRow`) has no label of
// its own — an unlabeled empty input reads as a chat box to type INTO, not a
// name to give something, which is exactly how a typed message became a
// branch named after it (caught live). The placeholder is the only
// affordance saying what this input is for, so it says it outright.
//
// Used to also say "or name/ for a folder" — retired with the OLD tree's own
// input, which created a folder for a trailing-slash name. This input only
// ever forks a branch; a slash in what's typed here just means a slash in
// the branch name, same as `feature/foo`.
export const CREATE_ROW_PLACEHOLDER = 'branch-name'

// ── Glyph weight: never pass strokeWidth to a Lucide row glyph ────────────────
// Measured across the live sidebar, every Lucide icon renders at Lucide's DEFAULT
// stroke of 2 on its 24-unit viewBox — 1.333px in a 16px box (the header's
// back/forward/settings cluster, SidebarToggleIcon, the New tab button) and
// 1.000px in a 12px `size-3` box (the repo row's Import branches). That default
// IS the house weight; matching it is a matter of passing no override at all.
//
// An override was tried and reverted: the hand-rolled 16-unit disclosure
// chevron above (DISCLOSURE_GLYPH_PATH) renders at 2/16 = 1.5px, and taking
// THAT as the reference makes every Lucide glyph beside it visibly the boldest
// mark in the sidebar. It is the outlier, not the standard. If the column is
// ever unified, move it onto Lucide rather than moving Lucide up to meet it.
//
// (Lucide's `absoluteStrokeWidth` is not the lever either: it computes from the
// `size` PROP, which class-sized glyphs never set, so it silently solves for 24px.)

// Trailing icon buttons on a row — repo settings, add-child, expand/collapse.
// These had drifted across FIVE different muted values inlined at each call site
// (text-foreground/30, /40, /50, /60, text-muted-foreground/40), so controls that
// should look identical rendered at visibly different weights.
//
// One token, full opacity, defined once. Use the muted token rather than a faded
// foreground: `text-foreground/30` is a transparency that composites differently
// over each surface (sidebar, hovered row, selected row), which is what made the
// drift visible in the first place. Never inline a faded variant.
export const ROW_SUB_ACTION =
  'inline-flex shrink-0 cursor-pointer rounded-lg p-1.5 text-muted-foreground ' +
  'hover:bg-sidebar-element-hover hover:text-foreground ' +
  'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

// Trailing action shown only when the row is hovered or focused — and taking
// NO space otherwise.
//
// `hidden`, so the box leaves the flex row entirely and the label measures
// against the whole row. Reserving the slot with `invisible` truncated a branch
// name at the same place whether or not anything was drawn there; floating the
// control out of flow instead fixed the width but painted the button ON TOP of
// the text it had just made room for.
//
// It costs a reflow of the row per hover transition, and that is affordable:
// toggling every row's control on every frame measures 9.03ms/frame against
// 8.57ms for a paint-only toggle — both at 120fps. (An earlier 149ms-vs-3ms
// reading came from forcing a synchronous layout 200 times in a tight loop,
// which is not what a hover does.)
//
// Deliberately NOT shown just because a row is the active/showing one (a
// `group-data-[active]:inline-flex` variant lived here briefly) — user
// correction, live: Recents' trailing action should appear on a real
// `:hover`/`:focus-within` only, active or not.
export const ROW_SUB_ACTION_HOVER =
  'hidden shrink-0 cursor-pointer rounded-lg p-1.5 text-muted-foreground ' +
  'group-hover:inline-flex group-focus-within:inline-flex ' +
  'hover:bg-sidebar-element-hover hover:text-foreground ' +
  'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

// `ROW_SUB_ACTION_HOVER`'s color half, re-keyed for a row painted on an
// INVERTED `ROW_ACTIVE` ground (`SidebarRowProps.activeGround` — a showing
// Recents row, solo or every member of a showing SET) — its hardcoded
// `text-muted-foreground`/`hover:bg-sidebar-element-hover` are both ambient-
// theme tokens, so on the OTHER theme's surface they land the wrong theme's
// colors on the wrong theme's background, same low-contrast bug `activeGround`
// already fixes for the row's own label/icon (SidebarRowProps' own doc) —
// just never applied to its trailing BUTTONS, which is what made "any button
// is not noticeable" once every button moved onto the row's own inline
// cluster where this ground actually shows through. Appended after
// `ROW_SUB_ACTION_HOVER` (never in place of it) so `cn`'s tailwind-merge
// dedupes only the conflicting color utilities and keeps every structural one
// (`hidden`, `p-1.5`, the `group-*:inline-flex` triad, focus ring) intact.
// The hover fill is a translucent wash of `--foreground-inverse` rather than
// reusing `--row-active-highlight` (a 1px top EDGE, not a fill) — the same
// "mix a low-opacity foreground over the background" shape every ordinary
// hover token already uses, just off the inverted pair so it lightens toward
// white on a dark ground and darkens toward black on a light one, in step
// with whichever theme is actually showing through `bg-background-inverse`.
export const ROW_SUB_ACTION_INVERTED =
  'text-foreground-inverse/70 hover:bg-foreground-inverse/10 hover:text-foreground-inverse'

// Every LEADING glyph on a row sits in this box. One label position per level:
// a box that differs by 2px between row types puts a visible wobble down the
// left edge wherever those rows interleave — which is exactly what folders and
// branches do. The repo avatar and the project row's slot are the two
// deliberate exceptions at 20px (h-5 w-5); they are the section headers.
export const ROW_GLYPH_BOX = 'inline-flex size-4 shrink-0 items-center justify-center'

// The second line under a branch name (change counts today). Muted TOKEN, never
// a faded foreground — see the ROW_SUB_ACTION note above: a transparency
// composites differently over the sidebar glass, the hover accent and the
// raised active surface, which is what made five different values look like
// five different controls.
//
// 10.5px with a tight leading so the two lines still fit the unchanged 36px
// row. `tabular-nums` keeps +12/-3 from jittering as the numbers tick.
export const ROW_SUBLABEL =
  'truncate font-mono text-[10.5px]/[13px] tabular-nums text-muted-foreground'

// The counts themselves keep the green/red they have always had — only the line
// they sit on is muted. Dropping the colour makes an insertion and a deletion
// distinguishable solely by a `+`/`-` glyph one pixel wide at this size, which
// is exactly the kind of at-a-glance signal the second line exists to preserve.
//
// Their own tokens rather than --git-added/--git-deleted: those resolve to
// emerald-500/red-500, which lose contrast against the raised active surface
// this line only ever appears on. See theme.css for the per-theme values.
export const ROW_SUBLABEL_ADD = 'text-sidebar-count-added'
export const ROW_SUBLABEL_DEL = 'text-sidebar-count-deleted'
