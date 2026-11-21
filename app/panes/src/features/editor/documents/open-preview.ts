import type { DocumentRef } from '@/lib/api/documents'
import { openDocument } from './open-document'

type PreviewOpener = (doc: DocumentRef) => Promise<void>

const page: { opener: PreviewOpener | null } = { opener: null }

/** Makes `opener` the way this page opens a document as the preview tab; null restores the page's own editor. */
export function installPreviewOpener(opener: PreviewOpener | null): void {
  page.opener = opener
}

/** Opens `doc` as the preview tab: in a shell pane the shell gives it a tab of its own, else this page's editor shows it. */
export async function openPreview(doc: DocumentRef): Promise<void> {
  if (page.opener) return page.opener(doc)
  await openDocument(doc, { preview: true })
}
