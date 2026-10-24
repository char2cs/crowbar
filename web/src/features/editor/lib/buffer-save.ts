/**
 * The ONE write path for editor buffers: content changes, manual save, Save
 * All and autosave all go through here.
 *
 * Invariants (pinned by __tests__/features/editor/lib/buffer-save.test.ts):
 *  - Autosave is debounced PER BUFFER: editing buffer B never cancels A's
 *    pending autosave.
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
import { documentRefOf } from '@/features/editor/documents/document-ref'
import {
  applyExternalText,
  documentText,
  markDocumentSaved,
  readDocumentText,
  replaceDocumentText,
  savedDocumentText,
} from '@/features/editor/documents/document-text'
import { findDocumentBuffer } from '@/features/editor/documents/open-document'
import { clearBlame } from '@/features/git/stores/git-blame-store'
import { requestGitRefresh } from '@/features/git/stores/git-refresh'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import { useSettingsStore } from '@/features/settings/store'
import { flagExternalChange } from '@/features/workspace/lib/external-buffer-sync'
import { toast } from '@/features/window/stores/toast-store'

/** Quiet period after the last edit before an autosave write. */
/** @internal Exported for unit tests. */
export const AUTOSAVE_DELAY_MS = 1000

const autosaveTimers = new Map<string, ReturnType<typeof setTimeout>>()
const saveChains = new Map<string, Promise<boolean>>()

/** openContent always gives an editor buffer a path; one without is ignored. */
export type PathedBuffer = EditorContent & { path: string }

export function findEditorBuffer(bufferId: string): PathedBuffer | null {
  const buffer = windowPaneStore.getState().buffers.find((b) => b.id === bufferId)
  return buffer && isEditorContent(buffer) && buffer.path ? (buffer as PathedBuffer) : null
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

function canAutosave(buffer: PathedBuffer): boolean {
  return !buffer.isVirtual && !buffer.path.startsWith('untitled:')
}

export function cancelAutosave(bufferId: string): void {
  const timer = autosaveTimers.get(bufferId)
  if (timer === undefined) return
  clearTimeout(timer)
  autosaveTimers.delete(bufferId)
}

function scheduleAutosave(bufferId: string): void {
  cancelAutosave(bufferId)
  autosaveTimers.set(
    bufferId,
    setTimeout(() => {
      autosaveTimers.delete(bufferId)
      void saveBuffer(bufferId, { reason: 'auto' })
    }, AUTOSAVE_DELAY_MS),
  )
}

/**
 * Record new text for a buffer (from Monaco's content sink or the markdown
 * editor) and arm that buffer's autosave when enabled.
 */
export function setBufferContent(bufferId: string, content: string): void {
  const buffer = findEditorBuffer(bufferId)
  if (!buffer) return
  if (buffer.content !== content) {
    patchEditorBuffer(bufferId, (b) => ({ ...b, content, isDirty: content !== b.savedContent }))
  }
  const next = findEditorBuffer(bufferId)
  if (next?.isDirty && canAutosave(next) && useSettingsStore.getState().settings.autoSave) {
    scheduleAutosave(bufferId)
  }
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

/** `holder` takes the untitled tab's place, unless both were edited while the write ran. */
function adoptSavedAs(untitled: PathedBuffer, holder: EditorContent, written: string): void {
  const ref = documentRefOf(holder)
  const typed = documentText(documentRefOf(untitled))
  const late = typed !== null && typed !== written ? typed : null
  if (documentText(ref) !== savedDocumentText(ref)) {
    // Edited during the write: the rule for any disk change under unsaved edits.
    flagExternalChange(ref)
    // Both moved on: the untitled tab stays, holding its late typing.
    if (late !== null) return
  } else {
    applyExternalText(ref, written, { saved: true })
    // Edits typed into the untitled tab while the write ran stay, unsaved.
    if (late !== null) replaceDocumentText(ref, late)
  }
  patchEditorBuffer(holder.id, (b) => ({ ...b, isPreview: false }))
  windowPaneStore.getState().paneActions.replaceEditorTab(untitled.id, holder.id)
}

async function saveAsUntitled(buffer: PathedBuffer, content: string): Promise<boolean> {
  const target = window.prompt('Save as:', buffer.name)
  if (!target) return false
  // Buffers are window-level — write to THIS buffer's own workspace, never the
  // merely-active one.
  const ref = workspaceRef(buffer.workspaceId, target)
  const open = findDocumentBuffer(ref)
  if (open && documentText(ref) !== savedDocumentText(ref)) {
    toast.error(`Save or discard changes in ${open.name} first`)
    return false
  }
  try {
    await writeDocument(ref, content)
  } catch (error) {
    console.error('Error saving file:', error)
    reportSaveError(error)
    return false
  }
  // Decided after the write, so an open that landed during it is seen too.
  const holder = findDocumentBuffer(ref)
  if (holder) {
    adoptSavedAs(buffer, holder, content)
    return true
  }
  markDocumentSaved(documentRefOf(buffer), content)
  patchEditorBuffer(buffer.id, (b) => ({
    ...b,
    path: target,
    name: target.split('/').pop() || target,
    isVirtual: false,
  }))
  return true
}

function saveVirtual(buffer: PathedBuffer, content: string): boolean {
  const ref = documentRefOf(buffer)
  if (buffer.path === 'settings://user-settings.json') {
    const ok = useSettingsStore.getState().updateSettingsFromJSON(content)
    if (ok) markDocumentSaved(ref, content)
    return ok
  }
  markDocumentSaved(ref, content)
  return true
}

async function writeToDisk(
  bufferId: string,
  { reason }: { reason: 'manual' | 'auto' },
): Promise<boolean> {
  const opened = findEditorBuffer(bufferId)
  if (!opened) return false
  const ref = documentRefOf(opened)
  const text = await readDocumentText(ref)
  const buffer = findEditorBuffer(bufferId)
  if (!buffer) return false
  if (buffer.path.startsWith('untitled:'))
    return reason === 'manual' && saveAsUntitled(buffer, text)
  if (buffer.isVirtual) return saveVirtual(buffer, text)
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
  const after = findEditorBuffer(bufferId)
  if (after?.isDirty && useSettingsStore.getState().settings.autoSave) scheduleAutosave(bufferId)
  return true
}

/**
 * Save one buffer. Saves of the same buffer run strictly one after another.
 * Resolves true when the buffer's content reached disk (or its virtual owner).
 */
export function saveBuffer(
  bufferId: string,
  options: { reason: 'manual' | 'auto' } = { reason: 'manual' },
): Promise<boolean> {
  if (options.reason === 'manual') cancelAutosave(bufferId)
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
