export type AttachmentLangKind = 'text-attachment' | 'excalidraw'

export interface ParsedAttachmentLang {
  kind: AttachmentLangKind
  id: string
}

const ATTACHMENT_ID_PATTERN = /^[A-Za-z0-9_-]{6,}$/

/** A bare tag without a valid id deliberately does not match, so a fence that
 *  merely discusses the feature renders as a plain code block. */
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
