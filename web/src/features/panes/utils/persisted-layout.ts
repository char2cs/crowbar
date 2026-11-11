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

/** A native webview id; Rust names the webview `browser-<id>`. */
const WEBVIEW_ID = /^[A-Za-z0-9_-]+$/

function isRestorable(buffer: PaneContent): boolean {
  if (!PANE_CONTENT_TYPES.has(buffer.type)) return false
  return buffer.type !== 'browser' || (WEBVIEW_ID.test(buffer.id) && typeof buffer.url === 'string')
}

/** Strips what belongs to this session alone: a live webview's in-flight load, and a failed
 *  read's reason (the boot restore reads the file again). */
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

/** The buffers as they are written to the saved layout. */
export function toPersistedBuffers(buffers: PaneContent[]): PaneContent[] {
  return buffers.map(withoutTransient)
}

interface PersistablePane {
  editorTabIds: string[]
  activeEditorTabId: string | null
}

// Generic over the pane type so a real `Record<string, PaneGroup>` round-trips
// unchanged in every field this function does not touch.
export interface Snapshot<P extends PersistablePane = PersistablePane> {
  buffers: PaneContent[]
  panes: Record<string, P>
}

interface Adoption {
  /** The pane that takes a kept buffer no pane lists. */
  adoptInto: string
  /** Keep every unlisted buffer, not only unsaved ones (a record with no views). */
  keepUnlisted?: boolean
}

/** The only check of persisted buffers: unknown content types are dropped, then C2 is set up once
 *  — dangling tab ids leave their pane, unlisted buffers are dropped unless they hold unsaved
 *  edits (`adoptInto` takes those). The store keeps C2 after that. */
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

/** Buffers saved before a buffer named its document: an editor's file and a preview's source
 *  were always its workspace's. */
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

/** Layouts saved while buffers carried their text: unsaved text becomes a draft and each buffer
 *  keeps only its identity. Runs after upgradeLegacyBuffers, so every editor names its document. */
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
