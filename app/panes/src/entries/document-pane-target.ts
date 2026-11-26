import { workspaceRef, type DocumentRef } from '@/lib/api/documents'

type DocumentPreview = 'markdown' | 'html'

/** What a file pane's page shows: the document, and the preview it renders it as, or null for its rich editor. */
export interface DocumentPaneTarget {
  doc: DocumentRef
  preview: DocumentPreview | null
}

const PREVIEWS: readonly DocumentPreview[] = ['markdown', 'html']

function previewOf(value: unknown): DocumentPreview | null {
  return PREVIEWS.find((preview) => preview === value) ?? null
}

/** Reads the state the shell saved for a file pane of workspace `wsId`; null when it names no file. */
export function documentPaneTarget(wsId: string, saved: unknown): DocumentPaneTarget | null {
  if (typeof saved !== 'object' || saved === null) return null
  const { path, absolute, preview } = saved as Record<string, unknown>
  if (typeof path !== 'string' || path === '') return null
  const doc: DocumentRef =
    absolute === true ? { authority: 'host', path } : workspaceRef(wsId, path)
  return { doc, preview: previewOf(preview) }
}
