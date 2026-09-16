import { useCallback } from 'react'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { editorAPI } from '@/features/editor/extensions/api'
import { useCenterCursor } from '@/features/editor/hooks/use-center-cursor'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { calculateOffsetFromContentPosition } from '@/features/editor/utils/position'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import { toast } from '@/features/window/stores/toast-store'
import { logger } from '../utils/logger'
import type { EditorCoordinateResolver } from '../view-model/view-layout'

// The daemon answers with WORKSPACE-RELATIVE paths — it owns the worktree root
// and relativizes the language server's absolute file:// URIs before replying —
// which is the form buffers are keyed by and the files API accepts. The one
// exception is a target the worktree does not contain (a stdlib or module-cache
// source), which has no relative form and arrives absolute.
interface Definition {
  filePath: string
  range: {
    start: { line: number; character: number }
    end: { line: number; character: number }
  }
}

function isOutsideWorkspace(filePath: string): boolean {
  return /^([A-Za-z]:)?[\\/]/.test(filePath)
}

interface UseGoToDefinitionProps {
  getDefinition?: (
    filePath: string,
    line: number,
    character: number,
  ) => Promise<Definition[] | null>
  isLanguageSupported?: (filePath: string) => boolean
  filePath: string
  lineHeight: number
  charWidth: number
  resolveEditorPosition?: EditorCoordinateResolver
}

export const useGoToDefinition = ({
  getDefinition,
  isLanguageSupported,
  filePath,
  lineHeight,
  charWidth,
  resolveEditorPosition,
}: UseGoToDefinitionProps) => {
  const { centerCursorInViewport } = useCenterCursor()

  const handleClick = useCallback(
    async (e: React.MouseEvent<HTMLDivElement>) => {
      // Only handle Cmd+Click (Mac) or Ctrl+Click (Windows/Linux)
      if (!e.metaKey && !e.ctrlKey) {
        return
      }

      if (!getDefinition || !isLanguageSupported?.(filePath || '')) {
        return
      }

      e.preventDefault()

      const editor = e.currentTarget
      if (!editor) return

      const rect = editor.getBoundingClientRect()
      const x = e.clientX - rect.left
      const y = e.clientY - rect.top

      // Get scroll from textarea (the actual scrollable element)
      const textarea = editor.querySelector('textarea')
      const scrollTop = textarea?.scrollTop ?? 0
      const scrollLeft = textarea?.scrollLeft ?? 0

      // Keep the fallback coordinate path aligned with editor content padding.
      const contentOffsetX = EDITOR_CONSTANTS.EDITOR_PADDING_LEFT
      const paddingTop = EDITOR_CONSTANTS.EDITOR_PADDING_TOP

      const resolvedPosition = resolveEditorPosition?.(e.clientX, e.clientY)
      const line = resolvedPosition?.line ?? Math.floor((y - paddingTop + scrollTop) / lineHeight)
      const character =
        resolvedPosition?.column ?? Math.floor((x - contentOffsetX + scrollLeft) / charWidth)

      if (line >= 0 && character >= 0) {
        try {
          logger.info('Editor', `Go to definition at ${filePath}:${line}:${character}`)
          const definitions = await getDefinition(filePath || '', line, character)

          if (definitions && definitions.length > 0) {
            const target = definitions[0]
            const targetFilePath = target.filePath

            const wsRef = getActiveWorkspaceStoreRef()
            const wsStore = wsRef?.getState()
            if (!wsStore) return
            // Task 26: panes/buffers are window-level now — `wsStore` only
            // still owns `workspaceId` (and other per-workspace slices);
            // pane/buffer state and actions come from the one window store.
            const paneStore = windowPaneStore.getState()

            // A target the workspace does not contain (a stdlib or dependency
            // source outside the worktree) can never be read through the
            // workspace-scoped files API, which rejects absolute paths. Say so
            // rather than issuing a request that is guaranteed to fail.
            if (isOutsideWorkspace(targetFilePath)) {
              logger.info('Editor', `Definition outside workspace: ${targetFilePath}`)
              toast.error('Definition is outside this workspace', targetFilePath)
              return
            }

            // Push current position to jump list before navigating
            const activeBufferId =
              paneStore.panes[paneStore.activePaneId]?.activeEditorTabId ?? null
            if (activeBufferId && filePath) {
              const editorState = useEditorStateStore.getState()
              useJumpListStore.getState().actions.pushEntry({
                bufferId: activeBufferId,
                // Workspace-relative paths are ambiguous across sibling
                // worktrees — name the workspace this position belongs to.
                workspaceId: wsStore.workspaceId,
                filePath,
                line: editorState.cursorPosition.line,
                column: editorState.cursorPosition.column,
                offset: editorState.cursorPosition.offset,
                scrollTop: editorState.scrollTop,
                scrollLeft: editorState.scrollLeft,
              })
            }
            // A pane renders only the buffers in its own editorTabIds, so activating
            // one it does not hold leaves it pointing at nothing and the editor
            // goes blank. Reveal in the pane that actually holds the buffer, and
            // attach to the active pane only when none does.
            const reveal = (bufferId: string): void => {
              const holdingPane = paneStore.paneActions.getPaneByEditorTabId(bufferId)
              const paneId = holdingPane?.id ?? paneStore.activePaneId
              if (holdingPane) {
                paneStore.paneActions.activateEditorTabInPane(paneId, bufferId)
              } else {
                // addEditorTabToPane always activates the tab it adds, so no
                // separate activate call is needed on this branch.
                const buf = paneStore.buffers.find((b) => b.id === bufferId)
                paneStore.paneActions.addEditorTabToPane(paneId, {
                  id: bufferId,
                  type: 'editor',
                  name: buf?.name ?? bufferId,
                  workspaceId: buf?.workspaceId ?? wsStore.workspaceId,
                })
              }
            }

            // Task 26: buffers are window-level — scope the lookup to THIS
            // workspace, since a sibling worktree can hold a different file
            // at the same relative path.
            const existingBuffer = paneStore.buffers.find(
              (b) => b.path === targetFilePath && b.workspaceId === wsStore.workspaceId,
            )

            if (existingBuffer) {
              reveal(existingBuffer.id)
            } else {
              // Read through this workspace, not whichever is active when the
              // await settles: linked worktrees of one repo share relative paths.
              const content = await readWorkspaceFile(wsStore.workspaceId, targetFilePath)
              const fileName = targetFilePath.split('/').pop() || 'untitled'
              const bufferId = paneStore.bufferActions.openContent({
                type: 'editor',
                path: targetFilePath,
                name: fileName,
                content,
                workspaceId: wsStore.workspaceId,
              })
              reveal(bufferId)
            }

            // Set cursor position after buffer is ready
            setTimeout(() => {
              const offset = calculateOffsetFromContentPosition(
                editorAPI.getContent(),
                target.range.start.line,
                target.range.start.character,
              )

              editorAPI.setCursorPosition({
                line: target.range.start.line,
                column: target.range.start.character,
                offset,
              })

              requestAnimationFrame(() => {
                centerCursorInViewport(target.range.start.line)
              })

              logger.info(
                'Editor',
                `Jumped to ${targetFilePath}:${target.range.start.line}:${target.range.start.character}`,
              )
            }, 100)
          } else {
            logger.debug('Editor', 'No definition found')
          }
        } catch (error) {
          // A silent dead end is indistinguishable from "this symbol has no
          // definition". Log for diagnosis AND tell the user something failed.
          logger.error('Editor', 'Go to definition error:', error)
          toast.error(
            'Could not go to definition',
            error instanceof Error ? error.message : undefined,
          )
        }
      }
    },
    [
      getDefinition,
      isLanguageSupported,
      filePath,
      lineHeight,
      charWidth,
      centerCursorInViewport,
      resolveEditorPosition,
    ],
  )

  return {
    handleClick,
  }
}
