export const ROW_BASE =
  'flex cursor-pointer select-none items-center gap-2 rounded-lg border ' +
  'h-9 px-2 mx-1.5 my-0.5 text-[13px] font-medium outline-none ' +
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
