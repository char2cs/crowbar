let cached: number | null = null

/** Only a positive reading is cached: a probe before the webview settles can
 *  read 0 on a platform whose scrollbar reserves space. */
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
