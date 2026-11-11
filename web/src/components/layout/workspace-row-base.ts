// The capsule every sidebar row shares, minus its pitch (ROW_BASE adds 36px; file rows are denser).
// `pan-x pan-y`, not `none`: the space-switch wheel and the row-to-pane drag both need the axes.
// `contain-layout` scopes a fast flick's many hover reflows to each fixed-height row.
export const ROW_SHAPE =
  'flex cursor-pointer select-none items-center gap-1.5 rounded-lg border contain-layout ' +
  'px-1.5 text-[13px] font-medium outline-none [touch-action:pan-x_pan-y] ' +
  'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background'

export const ROW_BASE = `${ROW_SHAPE} h-9 mx-1.5 my-0.5`

// Flat, with no top-highlight: that gloss is reserved for the showing surface. Hover uses the same
// wash an open row rests on, so "something is here" reads as one signal.
export const ROW_INACTIVE = 'border-transparent text-foreground hover:bg-sidebar-element-idle'

// A view tab at rest: no fill, a hover wash. The raised TAB_ACTIVE surface replaces it.
export const TAB_IDLE =
  'border-transparent hover:bg-sidebar-element-hover hover:shadow-xs hover:shadow-black/10 ' +
  'hover:inset-shadow-[0_1px_var(--elevated-highlight)]'

// A row that has a view somewhere but is not on screen: a resting fill rather than muted text,
// because the row is open, not disabled.
export const ROW_HAS_VIEW_IDLE = 'bg-sidebar-element-idle'

// The nest signal: the row a drop would land INSIDE fills; a hairline (drop-indicator.tsx) means
// "between". Never both, and a fill rather than a ring, which reads as focus. Own tokens rather
// than --primary so it reads over a hovered or active row without shouting (values in theme.css).
export const ROW_NEST_TARGET = 'border-sidebar-drop-nest-edge bg-sidebar-drop-nest text-foreground'

// The disclosure chevron every expandable row draws in its trailing slot, rotated 90° when open.
export const DISCLOSURE_GLYPH_PATH = 'M6 3l5 5-5 5'

// One tree level, in px. Every indented wrapper steps by this, so a row hoisted
// under a folded parent lands exactly one step in.
export const ROW_INDENT_STEP = 14

// A row kept through a collapse is re-drawn one step under its holding parent, so its indent
// animates. `margin-inline-start` so the row's own box moves, not the column around it.
export const ROW_INDENT_TRANSITION = 'motion-safe:[transition:margin-inline-start_0.1s_ease-in-out]'

// The fork-naming input has no label, and an empty unlabeled input reads as a chat box; the
// placeholder is its only affordance. A slash typed here is part of the branch name.
export const CREATE_ROW_PLACEHOLDER = 'branch-name'

// Glyph weight: never pass strokeWidth to a Lucide row glyph — its default stroke is the house
// weight. The 16-unit disclosure chevron above is the bolder outlier, and `absoluteStrokeWidth`
// computes from the `size` prop, which class-sized glyphs never set.

// Trailing icon buttons on a row. One muted TOKEN, never a faded foreground: a transparency
// composites differently over each surface, so identical controls render at different weights.
export const ROW_SUB_ACTION =
  'inline-flex shrink-0 cursor-pointer rounded-lg p-1.5 text-muted-foreground ' +
  'hover:bg-sidebar-element-hover hover:text-foreground ' +
  'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

// Shown only on real hover/focus (not merely on the active row) and taking NO space otherwise:
// `hidden` lets the label measure the whole row; `invisible` would truncate it, out-of-flow would
// paint over it. The per-hover row reflow measures as affordable.
export const ROW_SUB_ACTION_HOVER =
  'hidden shrink-0 cursor-pointer rounded-lg p-1.5 text-muted-foreground ' +
  'group-hover:inline-flex group-focus-within:inline-flex ' +
  'hover:bg-sidebar-element-hover hover:text-foreground ' +
  'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

// ROW_SUB_ACTION_HOVER's colours re-keyed for an inverted ground (`activeGround`), where ambient
// tokens lose contrast. Append after it (never instead) so tailwind-merge swaps only the colours.
export const ROW_SUB_ACTION_INVERTED =
  'text-foreground-inverse/70 hover:bg-foreground-inverse/10 hover:text-foreground-inverse'

// Every LEADING glyph on a row sits in this box, so interleaved row types share one label edge.
// The repo avatar and project slot are the deliberate 20px exceptions: they are section headers.
export const ROW_GLYPH_BOX = 'inline-flex size-4 shrink-0 items-center justify-center'

// The second line under a branch name. Muted token (see ROW_SUB_ACTION); sized and leaded so both
// lines fit the 36px row; `tabular-nums` keeps +12/-3 from jittering as the numbers tick.
export const ROW_SUBLABEL =
  'truncate font-mono text-[10.5px]/[13px] tabular-nums text-muted-foreground'

// The counts keep green/red so +/- is not told apart by a 1px glyph alone. Own tokens because
// --git-added/--git-deleted lose contrast on the raised active surface (values in theme.css).
export const ROW_SUBLABEL_ADD = 'text-sidebar-count-added'
export const ROW_SUBLABEL_DEL = 'text-sidebar-count-deleted'
