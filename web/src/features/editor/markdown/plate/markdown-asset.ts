import { createContext, useContext } from 'react'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import {
  isImagePath,
  isSelfLoading,
  mimeForPath,
  resolveAssetPath,
  toDataUrl,
} from '@/features/editor/lib/asset-data-url'

/**
 * Where a markdown buffer lives, so local image references inside it can be
 * resolved and loaded. Provided by `MarkdownEditorPane`; consumed by the `html`
 * node (and any other renderer that shows local assets). Null when unknown
 * (e.g. a standalone unit-test render) — resolution is then skipped and images
 * keep their raw src.
 */
export interface MarkdownAssetInfo {
  /** Workspace id the file belongs to (for `readWorkspaceFile`, or a `resolve`
   *  override that needs it). */
  wsId: string
  /** The file's own directory, workspace-relative ('' = workspace root).
   *  Unused when `resolve` is set. */
  fileDir: string
  /** Override the default fileDir-relative `readWorkspaceFile` resolution.
   *  When present, `loadLocalImage` calls this directly with the raw `src`
   *  instead — the chat asset context (`chat-asset-resolver.ts`) uses this to
   *  route through the attachment-serving endpoint, since a chat-attachment
   *  ref isn't a workspace-relative path. */
  resolve?: (src: string) => Promise<string | null>
}

export const MarkdownAssetContext = createContext<MarkdownAssetInfo | null>(null)

export function useMarkdownAsset(): MarkdownAssetInfo | null {
  return useContext(MarkdownAssetContext)
}

export { resolveAssetPath }

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
  if (asset.resolve) return asset.resolve(src)
  const path = resolveAssetPath(asset.fileDir, src)
  const mime = mimeForPath(path)
  if (!mime || !isImagePath(path)) return null

  try {
    return toDataUrl(mime, await readWorkspaceFile(asset.wsId, path))
  } catch {
    return null
  }
}
