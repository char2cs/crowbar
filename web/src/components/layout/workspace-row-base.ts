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

// Shared add-child glyph (the thin "+" used on every row's trailing action) so
// the repo header's fork button is visually identical to a workspace row's.
export const ADD_GLYPH_PATH = 'M8 3v10M3 8h10'

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
