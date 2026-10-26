/**
 * Who owns a touch on a phone sidebar row. The hold arms at 250ms; from there the finger either
 * moves past CLAIM_PX (a reorder) or rests until the dwell (the row's menu). The page owns the
 * whole gesture; the shell only presents the menu it is asked for.
 */
export type RowTouch = 'idle' | 'armed' | 'drag' | 'menu'

export type RowTouchEvent =
  | { type: 'hold' }
  | { type: 'move'; dx: number; dy: number }
  | { type: 'dwell' }
  | { type: 'release' }

const CLAIM_PX = 8

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
