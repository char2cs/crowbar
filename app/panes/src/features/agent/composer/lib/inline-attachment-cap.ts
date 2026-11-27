const INLINE_ATTACHMENT_MAX_BYTES = 32 * 1024

export function exceedsInlineSizeCap(text: string): boolean {
  return new TextEncoder().encode(text).byteLength > INLINE_ATTACHMENT_MAX_BYTES
}
