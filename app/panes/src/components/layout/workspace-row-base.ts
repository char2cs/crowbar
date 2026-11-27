export const ROW_SHAPE =
  'flex cursor-pointer select-none items-center gap-1.5 rounded-lg border contain-layout ' +
  'px-1.5 text-[13px] font-medium outline-none [touch-action:pan-x_pan-y] ' +
  'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background'

export const ROW_BASE = `${ROW_SHAPE} h-9 mx-1.5 my-0.5`

export const ROW_INACTIVE = 'border-transparent text-foreground hover:bg-sidebar-element-idle'

export const TAB_IDLE =
  'border-transparent hover:bg-sidebar-element-hover hover:shadow-xs hover:shadow-black/10 ' +
  'hover:inset-shadow-[0_1px_var(--elevated-highlight)]'

export const ROW_HAS_VIEW_IDLE = 'bg-sidebar-element-idle'

export const ROW_NEST_TARGET = 'border-sidebar-drop-nest-edge bg-sidebar-drop-nest text-foreground'

export const DISCLOSURE_GLYPH_PATH = 'M6 3l5 5-5 5'

export const ROW_INDENT_STEP = 14

export const ROW_INDENT_TRANSITION = 'motion-safe:[transition:margin-inline-start_0.1s_ease-in-out]'

export const CREATE_ROW_PLACEHOLDER = 'branch-name'

export const ROW_SUB_ACTION =
  'inline-flex shrink-0 cursor-pointer rounded-lg p-1.5 text-muted-foreground ' +
  'hover:bg-sidebar-element-hover hover:text-foreground ' +
  'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

export const ROW_SUB_ACTION_HOVER =
  'hidden shrink-0 cursor-pointer rounded-lg p-1.5 text-muted-foreground ' +
  'group-hover:inline-flex group-focus-within:inline-flex ' +
  'hover:bg-sidebar-element-hover hover:text-foreground ' +
  'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

export const ROW_SUB_ACTION_INVERTED =
  'text-foreground-inverse/70 hover:bg-foreground-inverse/10 hover:text-foreground-inverse'

export const ROW_GLYPH_BOX = 'inline-flex size-4 shrink-0 items-center justify-center'

export const ROW_SUBLABEL =
  'truncate font-mono text-[10.5px]/[13px] tabular-nums text-muted-foreground'

export const ROW_SUBLABEL_ADD = 'text-sidebar-count-added'
export const ROW_SUBLABEL_DEL = 'text-sidebar-count-deleted'
