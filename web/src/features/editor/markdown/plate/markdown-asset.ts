import { createContext, useContext } from 'react'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'

/**
 * Where a markdown buffer lives, so local image references inside it can be
 * resolved and loaded. Provided by `MarkdownEditorPane`; consumed by the `html`
 * node (and any other renderer that shows local assets). Null when unknown
 * (e.g. a standalone unit-test render) — resolution is then skipped and images
 * keep their raw src.
 */
export interface MarkdownAssetInfo {
  /** Workspace id the file belongs to (for `readWorkspaceFile`). */
  wsId: string
  /** The file's own directory, workspace-relative ('' = workspace root). */
  fileDir: string
}

export const MarkdownAssetContext = createContext<MarkdownAssetInfo | null>(null)

export function useMarkdownAsset(): MarkdownAssetInfo | null {
  return useContext(MarkdownAssetContext)
}

const IMAGE_MIME: Record<string, string> = {
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  gif: 'image/gif',
  webp: 'image/webp',
  avif: 'image/avif',
  bmp: 'image/bmp',
  ico: 'image/x-icon',
  svg: 'image/svg+xml',
}

/** True for a src the browser can already load on its own. */
function isSelfLoading(src: string): boolean {
  return /^(https?:|data:|blob:)/i.test(src) || src.startsWith('//')
}

/**
 * Resolve a possibly-relative asset reference against the file's directory into
 * a workspace-relative path. `/foo` is workspace-root-relative; `foo`, `./foo`
 * and `../foo` are relative to `fileDir`. Strips `.`/`..` segments.
 */
export function resolveAssetPath(fileDir: string, src: string): string {
  const cleanSrc = src.split(/[?#]/)[0]
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

/**
 * Load a local image referenced from a markdown file as a `data:` URL the
 * webview can render, or `null` if it can't/shouldn't be resolved (remote URL,
 * non-image, unknown workspace, or a read error). Reads through the same
 * workspace file API the editor already uses; binary files come back as a
 * latin1 byte string (base64-decoded by the daemon layer), which re-encodes to
 * base64 cleanly. SVG is text, so it's passed through utf8 instead.
 */
export async function loadLocalImage(
  asset: MarkdownAssetInfo | null,
  src: string,
): Promise<string | null> {
  if (!asset || !src || isSelfLoading(src)) return null
  const path = resolveAssetPath(asset.fileDir, src)
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  const mime = IMAGE_MIME[ext]
  if (!mime) return null

  try {
    const bytes = await readWorkspaceFile(asset.wsId, path)
    if (ext === 'svg') {
      return `data:${mime};utf8,${encodeURIComponent(bytes)}`
    }
    return `data:${mime};base64,${btoa(bytes)}`
  } catch {
    return null
  }
}
