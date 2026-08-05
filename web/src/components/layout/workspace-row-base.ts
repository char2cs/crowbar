export const ROW_BASE =
  'flex cursor-pointer select-none items-center gap-1.5 rounded-lg border ' +
  'h-9 px-1.5 mx-1.5 my-0.5 text-[13px] font-medium outline-none ' +
  'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background'

// Shared selected/idle row variants so every sidebar row (workspaces, the repo
// header) renders an identical active state — a raised, inset-lit surface when
// selected and a flat hover-accent when not. Keep both in sync here, never inline.
export const ROW_ACTIVE =
  'border-background bg-background text-foreground shadow-xs shadow-black/10 ' +
  'not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)] ' +
  'active:inset-shadow-[0_1px_--theme(--color-black/8%)] active:shadow-none'

export const ROW_INACTIVE = 'border-transparent text-foreground hover:bg-accent'

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

// Shared add-child glyph (the thin "+" used on every row's trailing action) so
// the repo header's fork button is visually identical to a workspace row's.
export const ADD_GLYPH_PATH = 'M8 3v10M3 8h10'

// The disclosure chevron, drawn once. Every expandable row in the sidebar —
// project, repo, folder, workspace — closes with the same mark in the same
// trailing slot, rotated 90° when open. It used to be copy-pasted at four call
// sites, and the project row had drifted into a different gesture entirely (a
// LEADING glyph that swapped to a chevron on hover), which made the one control
// the tree repeats most often the one control it drew two ways. Render it
// through RowDisclosureButton (row-disclosure-button.tsx), never by hand.
export const DISCLOSURE_GLYPH_PATH = 'M6 3l5 5-5 5'

// The fold-away control on a row that is holding others: a bar with a chevron
// rising into it — fold up, into the parent. Two inward chevrons were tried and
// reverted; at 12px they close into an X, which beside a folder reads as delete.
export const FOLD_AWAY_GLYPH_BAR = 'M3.2 4h9.6'
export const FOLD_AWAY_GLYPH_CHEVRON = 'M4.8 11.2 8 8l3.2 3.2'

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

// One inline input covers both kinds of new row. A trailing slash means folder —
// the same thing it means everywhere else a path is typed — which keeps a second
// button and a right-click-only path off the row. The placeholder is the only
// affordance the rule has, so it says it outright.
export const CREATE_ROW_PLACEHOLDER = 'branch-name, or name/ for a folder'

// ── Glyph weight: never pass strokeWidth to a Lucide row glyph ────────────────
// Measured across the live sidebar, every Lucide icon renders at Lucide's DEFAULT
// stroke of 2 on its 24-unit viewBox — 1.333px in a 16px box (the header's
// back/forward/settings cluster, SidebarToggleIcon, the New tab button) and
// 1.000px in a 12px `size-3` box (the repo row's Import branches). That default
// IS the house weight; matching it is a matter of passing no override at all.
//
// An override was tried and reverted: the hand-rolled 16-unit SVGs below
// (ADD_GLYPH_PATH and the disclosure chevron) render at 2/16 = 1.5px, and taking
// THOSE as the reference makes every Lucide glyph beside them visibly the boldest
// mark in the sidebar. They are the outliers, not the standard. If the column is
// ever unified, move them onto Lucide rather than moving Lucide up to meet them.
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

// Non-interactive glyph leading a row (e.g. the "+" beside the inline create
// input). Same token as ROW_SUB_ACTION, without the button affordances.
export const ROW_SUB_ACTION_GLYPH = 'shrink-0 text-muted-foreground'

// Trailing action that keeps a permanent layout slot and becomes visible when
// the workspace/folder row is hovered or focused. Keeping the box in the flex
// row avoids resizing the label and re-laying out the branch on every pointer
// transition. Pointer events stay disabled while the control is invisible.
export const ROW_SUB_ACTION_HOVER =
  'invisible pointer-events-none inline-flex shrink-0 cursor-pointer rounded-lg p-1.5 text-muted-foreground ' +
  'group-hover:visible group-hover:pointer-events-auto ' +
  'group-focus-within:visible group-focus-within:pointer-events-auto ' +
  'hover:bg-sidebar-element-hover hover:text-foreground ' +
  'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

// The same rule, plus a fade as the control arrives — the fold-away button on a
// row that is holding others.
//
// The animation is attached to the hover/focus states so it begins when the
// permanently-mounted control becomes visible, instead of running invisibly on
// mount and being finished by the time the user reaches the row.
//
// Only this control fades. The add-child "+" is on every row of the tree at
// once, so a pointer travelling down the column would leave a trail of fades
// behind it; this one belongs to a single row that is in a state.
//
// The reduced-motion kill-switch in index.css zeroes the duration app-wide, so
// the control simply appears.
export const ROW_SUB_ACTION_FOLD_AWAY =
  ROW_SUB_ACTION_HOVER +
  ' group-hover:animate-row-action-in group-focus-within:animate-row-action-in'

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
