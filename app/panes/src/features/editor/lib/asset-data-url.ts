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

export function toDataUrl(mime: string, bytes: string): string {
  return TEXT_MIME.test(mime)
    ? `data:${mime};charset=utf-8,${encodeURIComponent(bytes)}`
    : `data:${mime};base64,${btoa(bytes)}`
}

export function textFromDataUrl(url: string): string | null {
  const header = /^data:([^;,]+);charset=utf-8,/.exec(url)
  if (!header?.[1] || !TEXT_MIME.test(header[1])) return null
  try {
    return decodeURIComponent(url.slice(header[0].length))
  } catch {
    return null
  }
}

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

export function isSelfLoading(src: string): boolean {
  return (
    src === '' || src.startsWith('#') || src.startsWith('//') || /^[a-z][a-z\d+.-]*:/i.test(src)
  )
}
