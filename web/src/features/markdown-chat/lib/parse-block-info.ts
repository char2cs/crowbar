// Parses a fenced-code info string into a structured descriptor.
//
// The info string is everything after the opening ``` (already stripped of the
// backticks). Examples:
//   "typescript"                  -> { type: 'typescript', params: {},  meta: '' }
//   "excalidraw widget-id:abc"    -> { type: 'excalidraw', params: { 'widget-id': 'abc' }, meta: 'widget-id:abc' }
//   "ts {1,3-5}"                  -> { type: 'ts', params: {}, meta: '{1,3-5}' }
//
// This is the SINGLE place fence info strings are parsed, so the embed syntax
// (fences today, possibly `:::` directives later) can evolve without touching
// any renderer.

export interface BlockInfo {
  /** First token — the language or block type. */
  type: string
  /** `key:value` tokens after the type (e.g. widget-id:abc). */
  params: Record<string, string>
  /** Everything after the type, verbatim (e.g. line-highlight hints). */
  meta: string
}

export function parseBlockInfo(info: string): BlockInfo {
  const trimmed = info.trim()
  if (trimmed === '') return { type: '', params: {}, meta: '' }

  const [type = '', ...rest] = trimmed.split(/\s+/)
  const meta = rest.join(' ')
  const params: Record<string, string> = {}
  for (const token of rest) {
    const sep = token.indexOf(':')
    if (sep > 0) params[token.slice(0, sep)] = token.slice(sep + 1)
  }
  return { type, params, meta }
}
