/** A phone row touch: armed at 250ms, then past CLAIM_PX it is a reorder, or after the dwell the
 *  row's menu. The page owns the gesture; the shell only presents the menu asked for. */
export type RowTouch = 'idle' | 'armed' | 'drag' | 'menu'

export type RowTouchEvent =
  | { type: 'hold' }
  | { type: 'move'; dx: number; dy: number }
  | { type: 'dwell' }
  | { type: 'release' }

// Small enough that a slow, deliberate drag claims the touch well before the dwell elapses.
const CLAIM_PX = 4

/** How long an armed, resting finger waits before the menu opens. */
export const MENU_DWELL_MS = 250

export function rowTouch(state: RowTouch, event: RowTouchEvent): RowTouch {
  switch (event.type) {
    case 'hold':
      return state === 'idle' ? 'armed' : state
    case 'move':
      return state === 'armed' && Math.hypot(event.dx, event.dy) > CLAIM_PX ? 'drag' : state
    case 'dwell':
      return state === 'armed' ? 'menu' : state
    case 'release':
      return 'idle'
  }
}
