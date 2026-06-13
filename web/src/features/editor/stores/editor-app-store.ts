// Tauri invoke removed — remote SSH write is a no-op stub
import { create } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { extensionRegistry } from '@/extensions/registry/extension-registry'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useFileWatcherStore } from '@/features/file-system/controllers/file-watcher-store'
import { gitDiffCache } from '@/features/git/utils/git-diff-cache'
import {
  isEditorContent,
  type EditorContent,
  type PaneContent,
} from '@/features/panes/types/pane-content'
import { useSettingsStore } from '@/features/settings/store'
import { createSelectors } from '@/utils/zustand-selectors'
import { writeFile } from '@/features/file-system/controllers/platform'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { toast } from '@/features/window/stores/toast-store'
import type { Position, Range } from '../types/editor'
import { trackBufferHistoryChange } from './buffer-history-tracking'

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

async function saveEditorBufferById(bufferId: string): Promise<boolean> {
  // Managed Monaco editors are model-authoritative: keystrokes land in the model
  // and reach the Zustand buffer store through a throttled ContentSink (150ms
  // trailing flush). If the user types and saves within that window WITHOUT
  // blurring the editor, the last keystrokes are still pending in the sink and
  // not yet in the store. Synchronously dispatch a flush BEFORE reading
  // buffer.content below: the editor's listener calls sink.flush() synchronously,
  // and the sink's write updates the store synchronously, so getState() below
  // sees the latest content.
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new Event('flush-editor-content'))
  }

  const wsRef = getActiveWorkspaceStoreRef()
  const wsStore = wsRef?.getState()
  if (!wsStore) return false
  const { buffers } = wsStore
  const { updateSettingsFromJSON } = useSettingsStore.getState()
  const { markPendingSave } = useFileWatcherStore.getState()
  const activeBuffer = buffers.find((buffer) => buffer.id === bufferId)
  if (!activeBuffer || !isEditorContent(activeBuffer)) return false

  const markBufferDirty = (id: string, isDirty: boolean) => {
    wsRef?.setState((state) => ({
      buffers: state.buffers.map((b) =>
        b.id === id && isEditorContent(b)
          ? {
              ...b,
              isDirty,
              ...(isDirty ? {} : { savedContent: b.content, hasExternalChange: false }),
            }
          : b,
      ),
    }))
  }

  const updateBufferContent = (id: string, content: string, markDirty = true) => {
    wsRef?.setState((state) => ({
      buffers: state.buffers.map((b) =>
        b.id === id && isEditorContent(b)
          ? {
              ...b,
              content,
              ...(markDirty
                ? { isDirty: content !== b.savedContent }
                : { savedContent: content, isDirty: false }),
            }
          : b,
      ),
    }))
  }

  const updateBufferPath = (id: string, newPath: string) => {
    const newName = newPath.split('/').pop() || newPath
    wsRef?.setState((state) => ({
      buffers: state.buffers.map((b) =>
        b.id === id && isEditorContent(b)
          ? { ...b, path: newPath, name: newName, isVirtual: false, savedContent: b.content }
          : b,
      ),
    }))
  }

  if (activeBuffer.path.startsWith('untitled:')) {
    // No Tauri dialog available — prompt the user for a filename via browser API
    const result = window.prompt('Save as:', activeBuffer.name)
    if (!result) return false

    await writeFile(result, activeBuffer.content)
    updateBufferPath(activeBuffer.id, result)
    markBufferDirty(activeBuffer.id, false)
    return true
  }

  if (activeBuffer.isVirtual) {
    if (activeBuffer.path === 'settings://user-settings.json') {
      const success = updateSettingsFromJSON(activeBuffer.content)
      markBufferDirty(activeBuffer.id, !success)
      return success
    }

    markBufferDirty(activeBuffer.id, false)
    return true
  }

  if (activeBuffer.path.startsWith('remote://')) {
    // Remote SSH writes are out of scope for this session (no Tauri invoke)
    console.warn('Remote SSH file write is not supported in Crowbar web mode')
    markBufferDirty(activeBuffer.id, true)
    return false
  }

  try {
    markPendingSave(activeBuffer.path)

    let contentToSave = activeBuffer.content
    const { settings } = useSettingsStore.getState()

    if (settings.formatOnSave) {
      const { formatContent } = await import('@/features/editor/formatter/formatter-service')
      const languageId = extensionRegistry.getLanguageId(activeBuffer.path)

      const formatResult = await formatContent({
        filePath: activeBuffer.path,
        content: activeBuffer.content,
        languageId: languageId || undefined,
      })

      if (formatResult.success && formatResult.formattedContent) {
        contentToSave = formatResult.formattedContent
        updateBufferContent(activeBuffer.id, contentToSave, false)
      }
    }

    await writeFile(activeBuffer.path, contentToSave)
    const { LspClient } = await import('@/features/editor/lsp/lsp-client')
    await LspClient.getInstance().notifyDocumentSave(activeBuffer.path, contentToSave)
    markBufferDirty(activeBuffer.id, false)

    if (settings.lintOnSave) {
      const { lintContent } = await import('@/features/editor/linter/linter-service')
      const languageId = extensionRegistry.getLanguageId(activeBuffer.path)

      await lintContent({
        filePath: activeBuffer.path,
        content: contentToSave,
        languageId: languageId || undefined,
      })
    }

    const rootFolderPath = useFileSystemStore.getState().rootFolderPath
    if (rootFolderPath) {
      gitDiffCache.invalidate(rootFolderPath, activeBuffer.path)
      setTimeout(() => {
        window.dispatchEvent(
          new CustomEvent('git-status-updated', {
            detail: { filePath: activeBuffer.path },
          }),
        )
      }, 50)
    }
    return true
  } catch (error) {
    console.error('Error saving local file:', error)
    markBufferDirty(activeBuffer.id, true)
    reportSaveError(error)
    return false
  }
}

function getDirtyEditorBuffers(buffers: PaneContent[]): EditorContent[] {
  return buffers.filter(
    (buffer): buffer is EditorContent => isEditorContent(buffer) && buffer.isDirty,
  )
}

interface AppState {
  autoSaveTimeoutId: NodeJS.Timeout | null
  actions: AppActions
}

interface AppActions {
  handleContentChange: (
    content: string,
    previousContent?: string,
    previousCursorPosition?: Position,
    previousSelection?: Range,
    options?: { contentAlreadyApplied?: boolean; skipUndoGrouping?: boolean },
  ) => Promise<void>
  handleSave: () => Promise<void>
  handleSaveAll: () => Promise<number>
  cleanup: () => void
}

export const useEditorAppStore = createSelectors(
  create<AppState>()(
    immer((set, get) => ({
      autoSaveTimeoutId: null,
      actions: {
        handleContentChange: async (
          content: string,
          previousContent?: string,
          previousCursorPosition?: Position,
          previousSelection?: Range,
          options?: { contentAlreadyApplied?: boolean; skipUndoGrouping?: boolean },
        ) => {
          const wsRef = getActiveWorkspaceStoreRef()
          const wsStore = wsRef?.getState()
          if (!wsStore) return
          const { buffers, panes, activePaneId } = wsStore
          const activeBufferId = panes[activePaneId]?.activeBufferId ?? null
          const { settings } = useSettingsStore.getState()
          const { markPendingSave } = useFileWatcherStore.getState()
          const contentAlreadyApplied = options?.contentAlreadyApplied === true

          const activeBuffer = buffers.find((b) => b.id === activeBufferId)
          if (!activeBuffer || !isEditorContent(activeBuffer)) return

          if (activeBufferId) {
            trackBufferHistoryChange({
              bufferId: activeBufferId,
              currentContent: activeBuffer.content,
              nextContent: content,
              previousContent,
              previousCursorPosition,
              previousSelection,
              skipUndoGrouping: options?.skipUndoGrouping,
            })
          }

          const isRemoteFile = activeBuffer.path.startsWith('remote://')

          if (isRemoteFile) {
            if (!contentAlreadyApplied) {
              wsRef?.setState((state) => ({
                buffers: state.buffers.map((b) =>
                  b.id === activeBuffer.id && isEditorContent(b) ? { ...b, content } : b,
                ),
              }))
            }
          } else {
            if (!contentAlreadyApplied) {
              wsRef?.setState((state) => ({
                buffers: state.buffers.map((b) =>
                  b.id === activeBuffer.id && isEditorContent(b)
                    ? { ...b, content, isDirty: content !== b.savedContent }
                    : b,
                ),
              }))
            }

            if (!activeBuffer.isVirtual && settings.autoSave) {
              const { autoSaveTimeoutId } = get()
              if (autoSaveTimeoutId) {
                clearTimeout(autoSaveTimeoutId)
              }

              const newTimeoutId = setTimeout(async () => {
                try {
                  markPendingSave(activeBuffer.path)
                  await writeFile(activeBuffer.path, content)
                  wsRef?.setState((state) => ({
                    buffers: state.buffers.map((b) =>
                      b.id === activeBuffer.id && isEditorContent(b)
                        ? { ...b, isDirty: false, savedContent: content, hasExternalChange: false }
                        : b,
                    ),
                  }))

                  const rootFolderPath = useFileSystemStore.getState().rootFolderPath
                  if (rootFolderPath) {
                    gitDiffCache.invalidate(rootFolderPath, activeBuffer.path)
                    setTimeout(() => {
                      window.dispatchEvent(
                        new CustomEvent('git-status-updated', {
                          detail: { filePath: activeBuffer.path },
                        }),
                      )
                    }, 50)
                  }
                } catch (error) {
                  console.error('Error saving file:', error)
                  reportSaveError(error)
                  wsRef?.setState((state) => ({
                    buffers: state.buffers.map((b) =>
                      b.id === activeBuffer.id && isEditorContent(b) ? { ...b, isDirty: true } : b,
                    ),
                  }))
                }
              }, 150)

              set((state) => {
                state.autoSaveTimeoutId = newTimeoutId
              })
            }
          }
        },

        handleSave: async () => {
          const wsStore = getActiveWorkspaceStoreRef()?.getState()
          if (!wsStore) return
          const { buffers, panes, activePaneId } = wsStore
          const activeBufferId = panes[activePaneId]?.activeBufferId ?? null
          const activeBuffer = buffers.find((b) => b.id === activeBufferId)
          if (!activeBuffer || !isEditorContent(activeBuffer)) return

          await saveEditorBufferById(activeBuffer.id)
        },

        handleSaveAll: async () => {
          const wsStore = getActiveWorkspaceStoreRef()?.getState()
          if (!wsStore) return 0
          const dirtyBufferIds = getDirtyEditorBuffers(wsStore.buffers).map((buffer) => buffer.id)
          let savedCount = 0

          for (const bufferId of dirtyBufferIds) {
            const saved = await saveEditorBufferById(bufferId)
            const nextBuffer = getActiveWorkspaceStoreRef()
              ?.getState()
              .buffers.find((buffer) => buffer.id === bufferId)
            if (saved && (!nextBuffer || !isEditorContent(nextBuffer) || !nextBuffer.isDirty)) {
              savedCount += 1
            }
          }

          return savedCount
        },

        cleanup: () => {
          const { autoSaveTimeoutId } = get()
          if (autoSaveTimeoutId) {
            clearTimeout(autoSaveTimeoutId)
          }
        },
      },
    })),
  ),
)
