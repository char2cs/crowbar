/**
 * Who owns a touch on a phone sidebar row. The hold activates the drag sensor at 250ms; from
 * then the touch is either carried (the finger moved past CLAIM_PX before the menu showed) or
 * left to the native context menu (it stayed, and the system cancels the touch when the menu
 * appears).
 */
export type RowTouch = 'idle' | 'armed' | 'drag'

export type RowTouchEvent =
  { type: 'hold' } | { type: 'move'; dx: number; dy: number } | { type: 'release' }

const CLAIM_PX = 8

export function rowTouch(state: RowTouch, event: RowTouchEvent): RowTouch {
  switch (event.type) {
    case 'hold':
      return state === 'idle' ? 'armed' : state
    case 'move':
      return state === 'armed' && Math.hypot(event.dx, event.dy) > CLAIM_PX ? 'drag' : state
    case 'release':
      return 'idle'
  }
}
