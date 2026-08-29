let cached: number | null = null

/** The width a real (non-overlay) scrollbar reserves on this platform — a
 *  constant of the browser/OS, not of any one scrollable element, so one
 *  synthetic probe answers for every `.scroll` in the app. Cached: nothing
 *  here changes between the first call and the next. */
export function measureScrollbarWidth(): number {
  if (cached !== null) return cached
  const probe = document.createElement('div')
  probe.style.position = 'absolute'
  probe.style.top = '-9999px'
  probe.style.width = '100px'
  probe.style.height = '100px'
  probe.style.overflow = 'scroll'
  document.body.appendChild(probe)
  cached = probe.offsetWidth - probe.clientWidth
  probe.remove()
  return cached
}
