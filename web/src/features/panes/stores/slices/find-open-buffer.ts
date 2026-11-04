import type { DocumentRef } from '@/lib/api/documents'
import { documentRefOf, sameDocument } from '@/features/editor/documents/document-ref'
import {
  isEditorContent,
  type OpenEditorTabSpec,
  type PaneContent,
} from '@/features/panes/types/pane-content'

/** The workspace a document belongs to; '' for a host document. */
export const ownerOf = (ref: DocumentRef) => (ref.authority === 'workspace' ? ref.workspaceId : '')

/** The workspace a spec names; never the active one, so an open lands where its caller meant. */
export function specWorkspaceId(spec: OpenEditorTabSpec): string {
  switch (spec.type) {
    case 'editor':
      return ownerOf(spec.doc)
    case 'markdownPreview':
    case 'htmlPreview':
      return ownerOf(spec.source)
    case 'commitDiff':
    case 'branchReview':
      return spec.wsId
    default:
      return spec.workspaceId
  }
}

/**
 * The open buffer `spec` would duplicate — what `openContent` shows instead of
 * creating another. A document is one buffer whichever workspace asks; a New
 * Tab page is unique per pane only (`paneTabIds`).
 */
export function findOpenBuffer(
  buffers: readonly PaneContent[],
  spec: OpenEditorTabSpec,
  paneTabIds: readonly string[],
): PaneContent | undefined {
  switch (spec.type) {
    case 'editor':
      return buffers.find((b) => isEditorContent(b) && sameDocument(documentRefOf(b), spec.doc))
    case 'markdownPreview':
    case 'htmlPreview':
      return buffers.find(
        (b) =>
          (b.type === 'markdownPreview' || b.type === 'htmlPreview') &&
          b.type === spec.type &&
          sameDocument(b.source, spec.source),
      )
    case 'branchReview':
      return buffers.find((b) => b.type === 'branchReview' && b.wsId === spec.wsId)
    case 'commitDiff':
      return buffers.find(
        (b) => b.type === 'commitDiff' && b.wsId === spec.wsId && b.sha === spec.sha,
      )
    case 'terminal':
      if (!spec.sessionId) return undefined
      return buffers.find((b) => b.type === 'terminal' && b.sessionId === spec.sessionId)
    case 'newTabPage': {
      const inPane = new Set(paneTabIds)
      return buffers.find((b) => b.type === 'newTabPage' && inPane.has(b.id))
    }
    default:
      // A browser tab is a fresh webview every time.
      return undefined
  }
}
