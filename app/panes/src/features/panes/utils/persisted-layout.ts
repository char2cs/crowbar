import {
  PANE_CONTENT_TYPES,
  hasUnsavedEdits,
  type EditorContent,
  type PaneContent,
} from '@/features/panes/types/pane-content'
import { workspaceRef } from '@/lib/api/documents'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import type { EditorDraft } from '@/features/editor/documents/draft-store'
import type { WorkspaceLayout } from '@/lib/persistence/schemas'

const WEBVIEW_ID = /^[A-Za-z0-9_-]+$/

function isRestorable(buffer: PaneContent): boolean {
  if (!PANE_CONTENT_TYPES.has(buffer.type)) return false
  return buffer.type !== 'browser' || (WEBVIEW_ID.test(buffer.id) && typeof buffer.url === 'string')
}

function withoutTransient(buffer: PaneContent): PaneContent {
  if (buffer.type === 'editor' && buffer.unavailable) {
    const { unavailable: _reason, ...kept } = buffer
    return kept
  }
  if (buffer.type !== 'browser' || !('loading' in buffer)) return buffer
  const rest = { ...buffer }
  delete rest.loading
  return rest
}

export function toPersistedBuffers(buffers: PaneContent[]): PaneContent[] {
  return buffers.map(withoutTransient)
}

interface PersistablePane {
  editorTabIds: string[]
  activeEditorTabId: string | null
}

export interface Snapshot<P extends PersistablePane = PersistablePane> {
  buffers: PaneContent[]
  panes: Record<string, P>
}

interface Adoption {
  adoptInto: string
  keepUnlisted?: boolean
}

export function validateLoadedBuffers<P extends PersistablePane, T extends Snapshot<P>>(
  snapshot: T,
  { adoptInto, keepUnlisted = false }: Adoption,
): T {
  const known = snapshot.buffers.flatMap((b) => (isRestorable(b) ? [withoutTransient(b)] : []))
  const knownIds = new Set(known.map((b) => b.id))
  const panes = {} as Record<string, P>
  const listed = new Set<string>()
  for (const [paneId, pane] of Object.entries(snapshot.panes) as [string, P][]) {
    const editorTabIds = pane.editorTabIds.filter((id) => knownIds.has(id))
    for (const id of editorTabIds) listed.add(id)
    panes[paneId] = { ...pane, editorTabIds }
  }
  const adopted = known.filter(
    (b) => !listed.has(b.id) && (keepUnlisted || hasUnsavedEdits(b)) && panes[adoptInto],
  )
  if (adopted.length > 0) {
    const pane = panes[adoptInto]
    panes[adoptInto] = {
      ...pane,
      editorTabIds: [...pane.editorTabIds, ...adopted.map((b) => b.id)],
    }
    for (const b of adopted) listed.add(b.id)
  }
  for (const [paneId, pane] of Object.entries(panes)) {
    const activeEditorTabId =
      pane.activeEditorTabId && pane.editorTabIds.includes(pane.activeEditorTabId)
        ? pane.activeEditorTabId
        : (pane.editorTabIds[0] ?? null)
    panes[paneId] = { ...pane, activeEditorTabId }
  }
  const buffers = known.filter((b) => listed.has(b.id))
  return { ...snapshot, buffers, panes }
}

export function upgradeLegacyBuffers(buffers: PaneContent[]): PaneContent[] {
  return buffers.map((buffer) => {
    const saved = buffer as unknown as Record<string, unknown>
    if (saved.type === 'editor' && !saved.authority) return { ...buffer, authority: 'workspace' }
    if ((saved.type === 'markdownPreview' || saved.type === 'htmlPreview') && !saved.source) {
      const { sourceFilePath, ...rest } = saved
      const path = typeof sourceFilePath === 'string' ? sourceFilePath : ''
      return { ...rest, source: workspaceRef(buffer.workspaceId, path) } as unknown as PaneContent
    }
    return buffer
  })
}

export function splitLegacyEditorContent(layout: WorkspaceLayout): {
  layout: WorkspaceLayout
  drafts: EditorDraft[]
} {
  if (!layout.buffers) return { layout, drafts: [] }
  const drafts: EditorDraft[] = []
  const buffers = layout.buffers.map((buffer) => {
    if (buffer.type !== 'editor') return buffer
    const {
      content,
      savedContent,
      tokens: _tokens,
      fileMissing: _fileMissing,
      ...kept
    } = buffer as EditorContent & {
      content?: unknown
      savedContent?: unknown
      tokens?: unknown
      fileMissing?: unknown
    }
    if (typeof content !== 'string' || content === savedContent) return kept
    const uri = documentUri(documentRefOf(kept))
    const savedText = typeof savedContent === 'string' ? savedContent : ''
    drafts.push({ uri, text: content, savedText, updatedAt: layout.updatedAt })
    return { ...kept, isDirty: true }
  })
  return { layout: { ...layout, buffers }, drafts }
}
