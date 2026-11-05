// The wheel goes to our scrollback or the app by the app's declared mouse tracking, not by buffer:
// a TUI can track the mouse on the primary buffer (DECSET 1003 without 1049), and a buffer test
// would swallow its wheel.

export function shouldScrollScrollback(
  event: Pick<WheelEvent, 'shiftKey'>,
  mouseTrackingMode: string | undefined,
  bufferType: 'normal' | 'alternate',
): boolean {
  // No mouse tracking → the terminal owns the wheel (scroll our scrollback / xterm native).
  if (!mouseTrackingMode || mouseTrackingMode === 'none') return false
  // Tracking on: the app owns the wheel on both buffers, except Shift+wheel on the primary buffer,
  // where there is scrollback to move.
  return event.shiftKey && bufferType !== 'alternate'
}
