// Decides who owns a mouse-wheel event over the terminal: our scrollback
// (intercept + scrollLines), or the running app (defer to xterm's built-in
// mouse forwarding). The decision is driven by the app's DECLARED mouse-tracking
// intent — NOT by which buffer (primary/alternate) is currently active.
//
// Why intent and not buffer type: the daemon legitimately serializes DECSET 1003
// (mouse tracking) WITHOUT 1049 (alt screen) whenever an app has mouse tracking
// active on the PRIMARY buffer — e.g. Claude Code's TUI, or any app that left the
// alt screen without first disabling mouse tracking. That yields the state
// (mouseTrackingMode === 'any', buffer.active.type === 'normal'). A buffer-type
// test — "intercept unless we're on the alternate buffer" — treats that state as
// "scroll our scrollback", so it preventDefault()s + stopPropagation()s the wheel
// (capture-phase, before xterm's bubble-phase forwarder) and the app never sees
// it: the wheel goes dead inside the TUI ("works for a moment, then stuck", mode
// stays 'any' the whole time). Routing by intent instead fixes this: if the app
// asked for mouse tracking, the app owns the wheel regardless of buffer type.

export function shouldScrollScrollback(
  event: Pick<WheelEvent, 'shiftKey'>,
  mouseTrackingMode: string | undefined,
  bufferType: 'normal' | 'alternate',
): boolean {
  // No mouse tracking → the terminal owns the wheel (scroll our scrollback / xterm native).
  if (!mouseTrackingMode || mouseTrackingMode === 'none') return false
  // Mouse tracking ON → the APP owns the wheel on BOTH buffers (defer to xterm's
  // forwarding). The one escape hatch is Shift+wheel — the universal "scroll the
  // terminal, not the app" modifier — and only on the primary buffer where there
  // is scrollback to move.
  return event.shiftKey && bufferType !== 'alternate'
}
