import { useFileWatcherStore } from '@/features/file-system/controllers/file-watcher-store'
import { workspaceRef, writeDocument, type DocumentRef } from '@/lib/api/documents'
import {
  documentName,
  documentRefOf,
  documentUri,
  isUntitledRef,
} from '@/features/editor/documents/document-ref'
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
import { report } from '@/lib/errors/report'
import { problemOf } from '@/lib/errors/problem'

/** @internal */
export const QUIET_PERIOD_MS = 1000

const quietTimers = new Map<string, ReturnType<typeof setTimeout>>()
const saveChains = new Map<string, Promise<boolean>>()

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

function reportSaveError(error: unknown): void {
  if (/locked/i.test(problemOf(error).message)) {
    toast.error('Workspace is read-only', 'Create or switch to a child workspace to edit files.')
    return
  }
  report(error, { action: 'Failed to save file' })
}

const isUntitled = (buffer: PathedBuffer) => buffer.isVirtual || isUntitledRef(buffer)

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
      writeDraft(uri)
      const buffer = bufferOfDocument(uri)
      if (buffer && !isUntitled(buffer) && useSettingsStore.getState().settings.autoSave) {
        void saveBuffer(buffer.id, { reason: 'auto' })
      }
    }, QUIET_PERIOD_MS),
  )
}

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

export function noteDocumentEdited(uri: string): void {
  if (!getEditorHost()?.documents.isDirty(uri)) return
  noteDraftChanged(uri)
  whenQuiet(uri)
}

async function formatBeforeSave(ref: DocumentRef, text: string): Promise<string | null> {
  const { formatDocumentWithLsp } = await import('@/features/editor/lsp/format-buffer')
  const formatted = await formatDocumentWithLsp(ref, text)
  if (formatted !== null && documentText(ref) === text) {
    applyExternalText(ref, formatted, { saved: false })
  }
  return documentText(ref)
}

function adoptSavedAs(untitled: PathedBuffer, holder: EditorContent, written: string): boolean {
  const ref = documentRefOf(holder)
  const typed = documentText(documentRefOf(untitled))
  const late = typed !== null && typed !== written ? typed : null
  const text = documentText(ref)
  if (text === written || text === savedDocumentText(ref)) {
    applyExternalText(ref, written, { saved: true })
    if (late !== null) replaceDocumentText(ref, late)
  } else {
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
  const asked = workspaceRef(buffer.workspaceId, target)
  const open = findDocumentBuffer(asked)
  if (open && documentText(asked) !== savedDocumentText(asked)) {
    toast.error(`Save or discard changes in ${open.name} first`)
    return false
  }
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
  const holder = findDocumentBuffer(ref)
  if (holder) return adoptSavedAs(buffer, holder, content)
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
  if (!getEditorHost()?.documents.has(documentUri(ref))) return false
  const text = await readDocumentText(ref)
  const buffer = findEditorBuffer(bufferId)
  if (!buffer) return false
  if (isUntitled(buffer)) return reason === 'manual' && saveAsUntitled(buffer, text)
  if (reason === 'auto' && !buffer.isDirty) return true

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

  noteDocumentEdited(documentUri(ref))
  return true
}

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
