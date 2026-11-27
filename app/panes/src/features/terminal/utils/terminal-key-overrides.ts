export function resolveKeyOverride(event: KeyboardEvent): string | null {
  if (event.type !== 'keydown' || event.metaKey || event.ctrlKey) return null
  if (event.key !== 'Enter') return null
  if (event.shiftKey) return '\x1b[13;2u'
  if (event.altKey) return '\x1b[13;3u'
  return null
}
