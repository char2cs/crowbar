const OPEN_SCREENS = 4
const PREFETCH_SCREENS = 2

const GUESSED_ROW_PX = 40
const MIN_WINDOW_ROWS = 12
const MAX_WINDOW_ROWS = 1000

export const PAGE_BYTES = 256 * 1024

export const openRows = (viewportPx: number) =>
  Math.min(
    MAX_WINDOW_ROWS,
    Math.max(MIN_WINDOW_ROWS, Math.ceil((OPEN_SCREENS * viewportPx) / GUESSED_ROW_PX)),
  )

export const nearsLoadedTop = (scrollTop: number, viewportPx: number) =>
  scrollTop <= viewportPx * PREFETCH_SCREENS
