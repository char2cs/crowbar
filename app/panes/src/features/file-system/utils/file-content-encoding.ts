/** The daemon's file-content payload (`GET /v0/documents/content`). `encoding` is 'base64' for
 *  a file the fs engine classed as binary, absent for UTF-8 text. Every reader of the route goes
 *  through {@link decodeFileContent}, or base64 gibberish renders as source. */
export interface FileContentPayload {
  content: string
  encoding?: string
}

/** The string the editor holds for a payload. `atob`, not a UTF-8 TextDecoder: the daemon only
 *  base64s bytes that are not valid UTF-8, so the honest form is atob's byte-per-code-unit
 *  (latin1) string. */
export function decodeFileContent(payload: FileContentPayload): string {
  if (payload.encoding === 'base64') return atob(payload.content)
  return payload.content
}
