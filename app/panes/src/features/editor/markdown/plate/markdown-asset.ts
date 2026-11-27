import { createContext, useContext } from 'react'
import { readDocument, type DocumentRef } from '@/lib/api/documents'
import { siblingRef } from '@/features/editor/documents/document-ref'
import {
  isImagePath,
  isSelfLoading,
  mimeForPath,
  toDataUrl,
} from '@/features/editor/lib/asset-data-url'

export type MarkdownAssetInfo =
  | { doc: DocumentRef }
  | { wsId: string; resolve: (src: string, signal?: AbortSignal) => Promise<string | null> }

export const MarkdownAssetContext = createContext<MarkdownAssetInfo | null>(null)

export function useMarkdownAsset(): MarkdownAssetInfo | null {
  return useContext(MarkdownAssetContext)
}

export async function loadLocalImage(
  asset: MarkdownAssetInfo | null,
  src: string,
  signal?: AbortSignal,
): Promise<string | null> {
  if (!asset || !src || isSelfLoading(src)) return null
  if ('resolve' in asset) return asset.resolve(src, signal)
  const ref = siblingRef(asset.doc, src)
  const mime = mimeForPath(ref.path)
  if (!mime || !isImagePath(ref.path)) return null

  try {
    return toDataUrl(mime, (await readDocument(ref)).text)
  } catch {
    return null
  }
}
