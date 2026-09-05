let cached: number | null = null

/** The width a real (non-overlay) scrollbar reserves on this platform — a
 *  constant of the browser/OS, not of any one scrollable element, so one
 *  synthetic probe answers for every `.scroll` in the app.
 *
 *  Only a POSITIVE reading is cached. A probe measured before the webview's
 *  own chrome has settled (seen live: the very first chat pane to mount, on
 *  a cold app launch) can read 0 even on a platform whose real scrollbar
 *  reserves space — and 0 is otherwise indistinguishable from a genuine
 *  overlay scrollbar, so caching it unconditionally let that one bad
 *  early reading stick for the rest of the session. A 0 just re-measures
 *  next call instead — cheap, synchronous, and self-correcting once the
 *  webview has actually settled. */
export function measureScrollbarWidth(): number {
  if (cached !== null) return cached
  const probe = document.createElement('div')
  probe.style.position = 'absolute'
  probe.style.top = '-9999px'
  probe.style.width = '100px'
  probe.style.height = '100px'
  probe.style.overflow = 'scroll'
  document.body.appendChild(probe)
  const width = probe.offsetWidth - probe.clientWidth
  probe.remove()
  if (width > 0) cached = width
  return width
}
