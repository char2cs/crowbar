// Above this, pasted text or an Excalidraw scene uploads as a file instead of inlining. Half the
// outbox's 64KB prompt ceiling (which refuses the draft outright), leaving room for the rest, so
// one paste or drawing cannot make a draft unsendable.
export const INLINE_ATTACHMENT_MAX_BYTES = 32 * 1024

/** Measured in UTF-8 bytes like the outbox's cap; `.length` counts UTF-16 units and
 *  undercounts non-ASCII text. */
export function exceedsInlineSizeCap(text: string): boolean {
  return new TextEncoder().encode(text).byteLength > INLINE_ATTACHMENT_MAX_BYTES
}
