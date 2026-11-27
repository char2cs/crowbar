export interface FileContentPayload {
  content: string
  encoding?: string
}

export function decodeFileContent(payload: FileContentPayload): string {
  if (payload.encoding === 'base64') return atob(payload.content)
  return payload.content
}
