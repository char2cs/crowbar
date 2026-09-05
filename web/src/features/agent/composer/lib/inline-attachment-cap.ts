/**
 * Shared byte-size ceiling for the inline-capable attachment kinds — pasted
 * text (text-attachment fence) and an Excalidraw scene (excalidraw fence).
 * Above this, content that would otherwise be embedded directly in the draft
 * uploads as a file attachment instead, so the draft carries a short markdown
 * link rather than the raw bytes.
 *
 * `MAX_PROMPT_TEXT_BYTES` (prompt-queue-persistence.ts) caps a whole queued
 * draft at 64KB with no fallback once tripped — the draft is simply refused
 * ("This prompt is too large to submit"). 32KB — half that ceiling — leaves
 * room for the rest of the message (surrounding prose, other attachments'
 * markdown, fence overhead) so a single oversized paste or drawing can no
 * longer produce an unsendable draft on its own.
 */
export const INLINE_ATTACHMENT_MAX_BYTES = 32 * 1024

/** UTF-8 byte length, the same measure `prompt-queue-persistence.ts`'s own
 *  `byteLength` uses — a JS string's `.length` counts UTF-16 code units,
 *  which undercounts anything outside the BMP and every non-ASCII cap this
 *  is meant to guard against. */
export function exceedsInlineSizeCap(text: string): boolean {
  return new TextEncoder().encode(text).byteLength > INLINE_ATTACHMENT_MAX_BYTES
}
