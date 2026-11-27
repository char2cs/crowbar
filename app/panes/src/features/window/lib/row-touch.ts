export type RowTouch = 'idle' | 'armed' | 'drag' | 'menu'

export type RowTouchEvent =
  | { type: 'hold' }
  | { type: 'move'; dx: number; dy: number }
  | { type: 'dwell' }
  | { type: 'release' }

const CLAIM_PX = 4

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
