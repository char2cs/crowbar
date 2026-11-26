import type { DocumentRef } from '@/lib/api/documents'

type PreviewOpener = (doc: DocumentRef) => Promise<void>

const page: { opener: PreviewOpener | null } = { opener: null }

/** Makes `opener` the way this page opens a document as the preview tab: the shell's own tab in a
 *  shell pane, the page's editor in the app. Kept out of this module so a pane never loads Monaco. */
export function installPreviewOpener(opener: PreviewOpener): void {
  page.opener = opener
}

/** Opens `doc` as the preview tab, through the opener the page installed at boot. */
export async function openPreview(doc: DocumentRef): Promise<void> {
  await page.opener?.(doc)
}
