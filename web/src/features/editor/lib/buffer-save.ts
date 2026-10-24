/**
 * The ONE write path for editor buffers: manual save, Save All and autosave
 * all go through here, and so do the dirty flag and the drafts.
 *
 * Invariants (pinned by __tests__/features/editor/lib/buffer-save.test.ts):
 *  - Edits are quiet-debounced PER DOCUMENT: editing B never delays A's
 *    draft or autosave.
 *  - A save marks a buffer clean only if the buffer still holds exactly the
 *    content that was written. Edits that land while the write is in flight
 *    keep the buffer dirty (and re-arm autosave).
 *  - Saves of the same buffer are serialized, so an older write can never
 *    finish last and record stale content as "saved".
 *  - Manual save and autosave share every side effect (didSave, git refresh,
 *    own-write echo suppression).
 */
import { useFileWatcherStore } from '@/features/file-system/controllers/file-watcher-store'
import { workspaceRef, writeDocument, type DocumentRef } from '@/lib/api/documents'
import { bestEffort } from '@/lib/best-effort'
import { documentName, documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import {
  applyExternalText,
  documentText,
  markDocumentSaved,
  readDocumentText,
  replaceDocumentText,
  savedDocumentText,
} from '@/features/editor/documents/document-text'
import { clearDraft, noteDraftChanged, writeDraft } from '@/features/editor/documents/draft-store'
import { documentModelOpener, findDocumentBuffer } from '@/features/editor/documents/open-document'
import { getEditorHost } from '@/features/editor/host/editor-host'
import { clearBlame } from '@/features/git/stores/git-blame-store'
import { requestGitRefresh } from '@/features/git/stores/git-refresh'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import { useSettingsStore } from '@/features/settings/store'
import { flagExternalChange } from '@/features/workspace/lib/external-buffer-sync'
import { toast } from '@/features/window/stores/toast-store'

/**
 * Quiet period after the last edit before a dirty document's draft and autosave are written.
 * @internal Exported for unit tests.
 */
export const QUIET_PERIOD_MS = 1000

const quietTimers = new Map<string, ReturnType<typeof setTimeout>>() // by document uri
const saveChains = new Map<string, Promise<boolean>>()

/** openContent always gives an editor buffer a path; one without is ignored. */
export type PathedBuffer = EditorContent & { path: string }

export function findEditorBuffer(bufferId: string): PathedBuffer | null {
  const buffer = windowPaneStore.getState().buffers.find((b) => b.id === bufferId)
  return buffer && isEditorContent(buffer) && buffer.path ? (buffer as PathedBuffer) : null
}

function bufferOfDocument(uri: string): PathedBuffer | null {
  const buffer = windowPaneStore
    .getState()
    .buffers.find((b) => isEditorContent(b) && b.path && documentUri(documentRefOf(b)) === uri)
  return buffer ? (buffer as PathedBuffer) : null
}

function patchEditorBuffer(bufferId: string, patch: (b: EditorContent) => EditorContent): void {
  windowPaneStore.setState((state) => ({
    buffers: state.buffers.map((b) => (b.id === bufferId && isEditorContent(b) ? patch(b) : b)),
  }))
}

// Surface save failures the user would otherwise never see. A locked workspace
// (protected/default branch) rejects writes with 409 — explain how to fix it.
function reportSaveError(error: unknown): void {
  const message = error instanceof Error ? error.message : String(error)
  if (/locked/i.test(message)) {
    toast.error('Workspace is read-only', 'Create or switch to a child workspace to edit files.')
    return
  }
  toast.error('Failed to save file', message)
}

const isUntitled = (buffer: PathedBuffer) => buffer.isVirtual || buffer.path.startsWith('untitled:')

/** Drops the draft write and autosave waiting on `uri`'s edits to pause. */
export function cancelPendingWrites(uri: string): void {
  const timer = quietTimers.get(uri)
  if (timer === undefined) return
  clearTimeout(timer)
  quietTimers.delete(uri)
}

function whenQuiet(uri: string): void {
  cancelPendingWrites(uri)
  quietTimers.set(
    uri,
    setTimeout(() => {
      quietTimers.delete(uri)
      bestEffort(writeDraft(uri), 'write draft')
      const buffer = bufferOfDocument(uri)
      if (buffer && !isUntitled(buffer) && useSettingsStore.getState().settings.autoSave) {
        void saveBuffer(buffer.id, { reason: 'auto' })
      }
    }, QUIET_PERIOD_MS),
  )
}

/** The document's dirty flag flipped: its buffer mirrors it, and a first edit keeps a preview. */
export function setDocumentDirty(uri: string, dirty: boolean): void {
  const buffer = bufferOfDocument(uri)
  if (buffer && buffer.isDirty !== dirty)
    patchEditorBuffer(buffer.id, (b) => ({ ...b, isDirty: dirty }))
  if (buffer && dirty && buffer.isPreview)
    windowPaneStore.getState().bufferActions.promotePreview(buffer.id)
  if (dirty) return
  cancelPendingWrites(uri)
  clearDraft(uri)
}

/** The document's text changed: once edits pause, a dirty one gets its draft and autosave. */
export function noteDocumentEdited(uri: string): void {
  if (!getEditorHost()?.documents.isDirty(uri)) return
  noteDraftChanged(uri)
  whenQuiet(uri)
}

/** The text to save once formatted; null when the document closed meanwhile. */
async function formatBeforeSave(ref: DocumentRef, text: string): Promise<string | null> {
  const { formatDocumentWithLsp } = await import('@/features/editor/lsp/format-buffer')
  const formatted = await formatDocumentWithLsp(ref, text)
  // An edit made while the formatter ran would be dropped by its stale result.
  if (formatted !== null && documentText(ref) === text) {
    applyExternalText(ref, formatted, { saved: false })
  }
  return documentText(ref)
}

/**
 * `holder` takes the untitled tab's place, unless both were edited while the
 * write ran: then the untitled tab keeps its unsaved text and this is false.
 */
function adoptSavedAs(untitled: PathedBuffer, holder: EditorContent, written: string): boolean {
  const ref = documentRefOf(holder)
  const typed = documentText(documentRefOf(untitled))
  const late = typed !== null && typed !== written ? typed : null
  const text = documentText(ref)
  if (text === written || text === savedDocumentText(ref)) {
    applyExternalText(ref, written, { saved: true })
    // Edits typed into the untitled tab while the write ran stay, unsaved.
    if (late !== null) replaceDocumentText(ref, late)
  } else {
    // Edited during the write: the rule for any disk change under unsaved edits.
    flagExternalChange(ref)
    if (late !== null) return false
  }
  patchEditorBuffer(holder.id, (b) => ({ ...b, isPreview: false }))
  windowPaneStore.getState().paneActions.replaceEditorTab(untitled.id, holder.id)
  return true
}

async function saveAsUntitled(buffer: PathedBuffer, content: string): Promise<boolean> {
  const target = window.prompt('Save as:', buffer.name)
  if (!target) return false
  // Buffers are window-level — write to THIS buffer's own workspace, never the
  // merely-active one.
  const asked = workspaceRef(buffer.workspaceId, target)
  const open = findDocumentBuffer(asked)
  if (open && documentText(asked) !== savedDocumentText(asked)) {
    toast.error(`Save or discard changes in ${open.name} first`)
    return false
  }
  // Ready before the write, so nothing can open the target between its write and its rebind.
  const openModel = await documentModelOpener()
  let ref: DocumentRef
  try {
    ref = await writeDocument(asked, content)
  } catch (error) {
    console.error('Error saving file:', error)
    reportSaveError(error)
    return false
  }
  if (ref.authority === 'workspace') clearBlame(ref.workspaceId, ref.path)
  // Decided after the write, so an open that landed during it is seen too.
  const holder = findDocumentBuffer(ref)
  if (holder) return adoptSavedAs(buffer, holder, content)
  // The untitled tab becomes the document; text typed during the write stays, unsaved.
  const untitled = documentUri(documentRefOf(buffer))
  openModel(ref, documentText(documentRefOf(buffer)) ?? content, content)
  windowPaneStore.getState().bufferActions.rebindEditorBuffer(buffer.id, ref, documentName(ref))
  getEditorHost()?.documents.release(untitled)
  clearDraft(untitled)
  const uri = documentUri(ref)
  setDocumentDirty(uri, getEditorHost()?.documents.isDirty(uri) ?? false)
  noteDocumentEdited(uri)
  return true
}

async function writeToDisk(
  bufferId: string,
  { reason }: { reason: 'manual' | 'auto' },
): Promise<boolean> {
  const opened = findEditorBuffer(bufferId)
  if (!opened) return false
  if (opened.unavailable) {
    toast.error(opened.unavailable.title, opened.unavailable.message)
    return false
  }
  const ref = documentRefOf(opened)
  // A tab whose document is still being restored has no text to write yet.
  if (!getEditorHost()?.documents.has(documentUri(ref))) return false
  const text = await readDocumentText(ref)
  const buffer = findEditorBuffer(bufferId)
  if (!buffer) return false
  if (isUntitled(buffer)) return reason === 'manual' && saveAsUntitled(buffer, text)
  // An autosave for a buffer a previous save already caught up is a no-op.
  if (reason === 'auto' && !buffer.isDirty) return true

  // A host document has no language server, watcher, git status or blame.
  const inWorkspace = ref.authority === 'workspace'
  const formats =
    inWorkspace && reason === 'manual' && useSettingsStore.getState().settings.formatOnSave
  const written = formats ? await formatBeforeSave(ref, text) : text
  if (written === null) return false

  const { path, workspaceId } = buffer
  try {
    if (inWorkspace) useFileWatcherStore.getState().markPendingSave(path)
    await writeDocument(ref, written)
  } catch (error) {
    console.error('Error saving file:', error)
    reportSaveError(error)
    return false
  }
  markDocumentSaved(ref, written)
  if (inWorkspace) {
    const { LspClient } = await import('@/features/editor/lsp/lsp-client')
    void LspClient.getInstance().documentSave(workspaceId, path)
    requestGitRefresh(workspaceId)
    clearBlame(workspaceId, path)
  }

  // Edits typed during the write keep the buffer dirty; make sure autosave
  // picks them up rather than leaving them waiting for the next keystroke.
  noteDocumentEdited(documentUri(ref))
  return true
}

/**
 * Save one buffer. Saves of the same buffer run strictly one after another.
 * Resolves true when the buffer's content reached disk.
 */
export function saveBuffer(
  bufferId: string,
  options: { reason: 'manual' | 'auto' } = { reason: 'manual' },
): Promise<boolean> {
  const previous = saveChains.get(bufferId) ?? Promise.resolve(true)
  const next = previous.then(
    () => writeToDisk(bufferId, options),
    () => writeToDisk(bufferId, options),
  )
  saveChains.set(bufferId, next)
  void next.finally(() => {
    if (saveChains.get(bufferId) === next) saveChains.delete(bufferId)
  })
  return next
}

export async function saveActiveBuffer(): Promise<boolean> {
  const { panes, activePaneId } = windowPaneStore.getState()
  const bufferId = panes[activePaneId]?.activeEditorTabId
  if (!bufferId || !findEditorBuffer(bufferId)) return false
  return saveBuffer(bufferId)
}

/**
 * Save every dirty editor buffer across all workspaces (buffers are one
 * window-level list; each save writes to its buffer's own workspace).
 * Sequential on purpose: an untitled buffer blocks on a Save-As prompt.
 */
export async function saveAllDirtyBuffers(): Promise<number> {
  const dirtyIds = windowPaneStore
    .getState()
    .buffers.filter((b): b is EditorContent => isEditorContent(b) && b.isDirty)
    .map((b) => b.id)
  let saved = 0
  for (const bufferId of dirtyIds) {
    // react-doctor-disable-next-line async-await-in-loop -- sequential: an untitled buffer blocks on a Save-As prompt.
    if (await saveBuffer(bufferId)) saved += 1
  }
  return saved
}
