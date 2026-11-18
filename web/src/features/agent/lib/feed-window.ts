/** How much of a chat the client holds, in screens: sized by what is seen, not a fixed row count. */
const OPEN_SCREENS = 4
/** The view asks for older rows once this near the first loaded one: under OPEN_SCREENS - 1, so
 *  a fresh open at the bottom is never near it. */
const PREFETCH_SCREENS = 2

const GUESSED_ROW_PX = 40
const MIN_WINDOW_ROWS = 12
const MAX_WINDOW_ROWS = 1000

/** The wire size one replay or older page may fill: about three screens of text, a few seconds
 *  on a slow link. The daemon cuts the bulky fields of what it sends within it to previews. */
export const PAGE_BYTES = 256 * 1024

/** Rows a fresh open replays, before any row's height is known. */
export const openRows = (viewportPx: number) =>
  Math.min(
    MAX_WINDOW_ROWS,
    Math.max(MIN_WINDOW_ROWS, Math.ceil((OPEN_SCREENS * viewportPx) / GUESSED_ROW_PX)),
  )

export const nearsLoadedTop = (scrollTop: number, viewportPx: number) =>
  scrollTop <= viewportPx * PREFETCH_SCREENS
