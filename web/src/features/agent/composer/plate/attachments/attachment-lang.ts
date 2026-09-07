export type AttachmentLangKind = 'text-attachment' | 'excalidraw'

export interface ParsedAttachmentLang {
  kind: AttachmentLangKind
  id: string
}

const ATTACHMENT_ID_PATTERN = /^[A-Za-z0-9_-]{6,}$/

/** Parses a code-block's `lang` (the fence's info string) into its attachment
 *  kind + id. A bare `text-attachment`/`excalidraw` tag with no id — or an id
 *  that's too short or has disallowed characters — deliberately does NOT
 *  match, so ordinary discussion of this feature (a fence in this repo's own
 *  docs, say) renders as a plain code block instead of being hijacked. */
export function parseAttachmentLang(lang: string | null | undefined): ParsedAttachmentLang | null {
  if (!lang) return null
  const colon = lang.indexOf(':')
  if (colon === -1) return null
  const kind = lang.slice(0, colon)
  const id = lang.slice(colon + 1)
  if (kind !== 'text-attachment' && kind !== 'excalidraw') return null
  if (!ATTACHMENT_ID_PATTERN.test(id)) return null
  return { kind, id }
}
