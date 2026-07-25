/**
 * The daemon's file-content payload (`GET .../files/content`).
 *
 * `encoding` is `'base64'` for a file the fs engine classified as BINARY
 * (engine/fs/internal/content: `isBinary(data)` → base64) and is omitted for
 * UTF-8 text. Every reader of that route must go through {@link decodeFileContent}
 * — a reader that ignores the field renders base64 gibberish as source, which is
 * exactly what `readWorkspaceFile` did while `openFileContent` decoded correctly.
 * One decode, one place, so the two paths cannot drift apart again.
 */
export interface FileContentPayload {
  content: string
  encoding?: string
}

/**
 * Decode a file-content payload into the string the editor holds.
 *
 * `atob` is deliberate rather than a UTF-8 TextDecoder: the daemon only base64s
 * payloads it decided are NOT valid UTF-8 text, so the honest client-side
 * representation is the byte-per-code-unit (latin1) string `atob` produces.
 */
export function decodeFileContent(payload: FileContentPayload): string {
  if (payload.encoding === 'base64') return atob(payload.content)
  return payload.content
}
