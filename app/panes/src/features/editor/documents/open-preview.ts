import type { DocumentRef } from '@/lib/api/documents'

type PreviewOpener = (doc: DocumentRef) => Promise<void>

const page: { opener: PreviewOpener | null } = { opener: null }

export function installPreviewOpener(opener: PreviewOpener): void {
  page.opener = opener
}

export async function openPreview(doc: DocumentRef): Promise<void> {
  await page.opener?.(doc)
}
