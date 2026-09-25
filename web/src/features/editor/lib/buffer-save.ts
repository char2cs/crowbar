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
import { writeWorkspaceFile } from '@/features/file-system/controllers/platform'
import { clearBlame } from '@/features/git/stores/git-blame-store'
import { requestGitRefresh } from '@/features/git/stores/git-refresh'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import { useSettingsStore } from '@/features/settings/store'
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

/** Pull any keystrokes Monaco's content sink is still coalescing into the store. */
function flushPendingEditorContent(): void {
  if (typeof window !== 'undefined') window.dispatchEvent(new Event('flush-editor-content'))
}

async function formatBeforeSave(buffer: PathedBuffer): Promise<void> {
  const { formatBufferWithLsp } = await import('@/features/editor/lsp/format-buffer')
  const formatted = await formatBufferWithLsp(buffer)
  if (formatted === null) return
  // Only apply if nothing changed while the formatter ran — otherwise the
  // formatted text is based on stale input and would drop those edits.
  const current = findEditorBuffer(buffer.id)
  if (!current || current.content !== buffer.content) return
  patchEditorBuffer(buffer.id, (b) => ({
    ...b,
    content: formatted,
    isDirty: formatted !== b.savedContent,
  }))
}

async function saveAsUntitled(buffer: PathedBuffer): Promise<boolean> {
  const target = window.prompt('Save as:', buffer.name)
  if (!target) return false
  const content = buffer.content
  // Buffers are window-level — write to THIS buffer's own workspace, never the
  // merely-active one.
  await writeWorkspaceFile(buffer.workspaceId, target, content)
  patchEditorBuffer(buffer.id, (b) => ({
    ...b,
    path: target,
    name: target.split('/').pop() || target,
    isVirtual: false,
    savedContent: content,
    isDirty: b.content !== content,
  }))
  return true
}

async function saveVirtual(buffer: PathedBuffer): Promise<boolean> {
  if (buffer.path === 'settings://user-settings.json') {
    const ok = useSettingsStore.getState().updateSettingsFromJSON(buffer.content)
    if (ok) markSaved(buffer.id, buffer.content)
    return ok
  }
  markSaved(buffer.id, buffer.content)
  return true
}

/** The buffer is clean only if it still holds exactly what was written. */
function markSaved(bufferId: string, written: string): void {
  patchEditorBuffer(bufferId, (b) => ({
    ...b,
    savedContent: written,
    isDirty: b.content !== written,
    hasExternalChange: b.content === written ? false : b.hasExternalChange,
  }))
}

async function writeToDisk(
  bufferId: string,
  { reason }: { reason: 'manual' | 'auto' },
): Promise<boolean> {
  flushPendingEditorContent()
  let buffer = findEditorBuffer(bufferId)
  if (!buffer) return false
  if (buffer.path.startsWith('untitled:')) return reason === 'manual' && saveAsUntitled(buffer)
  if (buffer.isVirtual) return saveVirtual(buffer)
  // An autosave for a buffer a previous save already caught up is a no-op.
  if (reason === 'auto' && !buffer.isDirty) return true

  if (reason === 'manual' && useSettingsStore.getState().settings.formatOnSave) {
    await formatBeforeSave(buffer)
    buffer = findEditorBuffer(bufferId)
    if (!buffer) return false
  }

  const { path, workspaceId } = buffer
  const written = buffer.content
  try {
    useFileWatcherStore.getState().markPendingSave(path)
    await writeWorkspaceFile(workspaceId, path, written)
  } catch (error) {
    console.error('Error saving file:', error)
    reportSaveError(error)
    return false
  }
  markSaved(bufferId, written)

  const { LspClient } = await import('@/features/editor/lsp/lsp-client')
  void LspClient.getInstance().documentSave(workspaceId, path)
  requestGitRefresh(workspaceId)
  clearBlame(workspaceId, path)

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
