/**
 * Local workspace files as `data:` URLs, for previews that render outside the
 * workspace's origin (markdown images, the sandboxed HTML preview). The files
 * API returns binary content as a latin1 byte string, which re-encodes to
 * base64 as-is; text types pass through as UTF-8.
 */
const MIME_BY_EXT: Record<string, string> = {
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  gif: 'image/gif',
  webp: 'image/webp',
  avif: 'image/avif',
  bmp: 'image/bmp',
  ico: 'image/x-icon',
  svg: 'image/svg+xml',
  apng: 'image/apng',
  tif: 'image/tiff',
  tiff: 'image/tiff',
  css: 'text/css',
  js: 'text/javascript',
  mjs: 'text/javascript',
  json: 'application/json',
  woff: 'font/woff',
  woff2: 'font/woff2',
  ttf: 'font/ttf',
  otf: 'font/otf',
  mp4: 'video/mp4',
  webm: 'video/webm',
  mp3: 'audio/mpeg',
  wav: 'audio/wav',
}

const TEXT_MIME = /^(text\/|image\/svg\+xml|application\/json)/

export function mimeForPath(path: string): string | null {
  const ext = path.split(/[?#]/)[0]?.split('.').pop()?.toLowerCase() ?? ''
  return MIME_BY_EXT[ext] ?? null
}

export function isImagePath(path: string): boolean {
  return mimeForPath(path)?.startsWith('image/') ?? false
}

/** `bytes` as returned by readWorkspaceFile (latin1 for binary files). */
export function toDataUrl(mime: string, bytes: string): string {
  return TEXT_MIME.test(mime)
    ? `data:${mime};charset=utf-8,${encodeURIComponent(bytes)}`
    : `data:${mime};base64,${btoa(bytes)}`
}

/**
 * Resolve a possibly-relative reference against a file's directory into a
 * workspace-relative path. `/foo` is workspace-root-relative; `foo`, `./foo`
 * and `../foo` are relative to `fileDir`. Strips `.`/`..` segments.
 */
export function resolveAssetPath(fileDir: string, src: string): string {
  const cleanSrc = src.split(/[?#]/)[0] ?? ''
  const segments = cleanSrc.startsWith('/')
    ? cleanSrc.split('/')
    : [...fileDir.split('/'), ...cleanSrc.split('/')]
  const out: string[] = []
  for (const seg of segments) {
    if (seg === '..') out.pop()
    else if (seg !== '.' && seg !== '') out.push(seg)
  }
  return out.join('/')
}

/** True for a reference the browser loads on its own (remote, data, blob, anchor). */
export function isSelfLoading(src: string): boolean {
  return (
    src === '' || src.startsWith('#') || src.startsWith('//') || /^[a-z][a-z\d+.-]*:/i.test(src)
  )
}
