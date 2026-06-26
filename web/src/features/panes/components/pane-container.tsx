import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  useBuffersByIds,
  useBufferActions,
} from '@/features/workspace/stores/hooks/use-buffer-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { stageHunk, unstageHunk } from '@/features/git/api/git-status-api'
import type { GitHunk } from '@/features/git/types/git-types'
import { useSettingsStore } from '@/features/settings/store'
import { buildPaneContentStyle } from '../utils/pane-border'
import { cn } from '@/lib/utils'
import { ROOT_PANE_POSITION, type PanePosition } from '../types/pane'
import TabBar from '@/features/tabs/components/tab-bar'
import { extractDroppedFilePaths } from '@/features/file-system/utils/file-system-dropped-paths'
import {
  clearInternalTabDragData,
  getInternalTabDragData,
  getInternalTabDragHover,
  resolveDropTarget,
} from '@/features/tabs/utils/internal-tab-drag'

import { EmptyEditorState } from './empty-editor-state'
import { BOTTOM_PANE_ID } from '../constants/pane'
import { useActivePaneId, usePaneActions } from '@/features/workspace/stores/hooks/use-pane-store'
import type { PaneGroup } from '../types/pane'
import type {
  BranchReviewContent,
  CrowbarChatContent,
  EditorContent,
  NewTabContent,
} from '../types/pane-content'
import {
  ensureBufferInPaneDropTarget,
  moveBufferToPaneDropTarget,
} from '../utils/pane-drop-actions'
import { getPaneSplitDropOptions } from '../utils/pane-drop-zones'
import { type DropZone, SplitDropOverlay } from './split-drop-overlay'

const ExternalEditorTerminal = lazy(() =>
  import('@/features/editor/components/external-editor-terminal').then((m) => ({
    default: m.ExternalEditorTerminal,
  })),
)
const MarkdownChatView = lazy(() =>
  import('@/features/markdown-chat/components/markdown-chat-view').then((m) => ({
    default: m.MarkdownChatView,
  })),
)
const BranchReviewPane = lazy(() =>
  import('@/features/git/components/branch-review-pane').then((m) => ({
    default: m.BranchReviewPane,
  })),
)
import { EditorPane } from './editor-pane'
import { TerminalPane } from './terminal-pane'
import { DiffPane } from './diff-pane'

interface PaneContainerProps {
  pane: PaneGroup
  position?: PanePosition
}

type EditorBufferShell = Pick<EditorContent, 'id' | 'path' | 'name' | 'type'>
type PaneRenderBuffer =
  | Exclude<import('../types/pane-content').PaneContent, EditorContent | NewTabContent>
  | EditorBufferShell

export function PaneContainer({ pane, position = ROOT_PANE_POSITION }: PaneContainerProps) {
  const activePaneId = useActivePaneId()
  const { activatePaneBuffer, setActivePane } = usePaneActions()
  const bufferActions = useBufferActions()
  const { closeBuffer: closeBufferForce } = bufferActions

  const handlePromote = useCallback(
    (bufferId: string) => {
      bufferActions.promotePreview(bufferId)
    },
    [bufferActions],
  )
  const workspaceStore = useWorkspaceStore()
  const openTerminalBuffer = (options?: {
    name?: string
    command?: string
    workingDirectory?: string
    remoteConnectionId?: string
    sessionId?: string
  }): string =>
    workspaceStore.getState().bufferActions.openContent({ type: 'terminal', ...options })
  const rootFolderPath = useFileSystemStore.use.rootFolderPath?.()
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
  const paneContentStyle = useMemo(
    () => buildPaneContentStyle(position, sidebarPosition),
    [position, sidebarPosition],
  )

  const [isDragOver, setIsDragOver] = useState(false)
  const [isTabDragOver, setIsTabDragOver] = useState(false)
  const [internalHoverZone, setInternalHoverZone] = useState<DropZone>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const isActivePane = pane.id === activePaneId

  const rawPaneBuffers = useBuffersByIds(pane.bufferIds)
  const paneBuffers = useMemo((): PaneRenderBuffer[] => {
    return rawPaneBuffers.flatMap((buffer) => {
      if (buffer.type === 'newTab') return []
      if (buffer.type === 'editor') {
        return [
          {
            id: buffer.id,
            path: buffer.path,
            name: buffer.name,
            type: buffer.type,
          } satisfies EditorBufferShell,
        ]
      }
      return [buffer as PaneRenderBuffer]
    })
  }, [rawPaneBuffers])

  const activeBuffer = useMemo(() => {
    if (!pane.activeBufferId) return null
    return paneBuffers.find((b) => b.id === pane.activeBufferId) || null
  }, [paneBuffers, pane.activeBufferId])

  const handlePaneClick = useCallback(() => {
    if (!isActivePane) {
      setActivePane(pane.id)
    }
  }, [isActivePane, pane.id, setActivePane])

  const handlePaneMouseDownCapture = useCallback(
    (e: React.MouseEvent) => {
      const target = e.target as HTMLElement
      const isEditorTextarea = target.classList.contains('editor-textarea')
      const isTerminalTextarea = target.classList.contains('xterm-helper-textarea')
      if (
        !isEditorTextarea &&
        !isTerminalTextarea &&
        target.closest("button, input, textarea, [role='button'], [role='menu']")
      ) {
        return
      }

      if (!isActivePane) {
        setActivePane(pane.id)
      }
    },
    [isActivePane, pane.id, setActivePane],
  )

  const handleTabClick = useCallback(
    (bufferId: string) => {
      // Update workspace pane store (new system)
      activatePaneBuffer(pane.id, bufferId)
      setActivePane(pane.id)
    },
    [pane.id, activatePaneBuffer, setActivePane],
  )

  const openFileTreeDropInPane = useCallback(
    async (
      fileDragData: { path: string; name: string; isDir: boolean },
      point: { x: number; y: number },
    ) => {
      if (fileDragData.isDir) return
      if (!handleFileOpen) return

      const target = resolveDropTarget(point)
      if (target.paneId !== pane.id) return

      // Use workspace store for split creation so the new pane renders correctly.
      const splitOptions = getPaneSplitDropOptions(target.zone)
      const targetPaneId = splitOptions
        ? (workspaceStore
            .getState()
            .paneActions.splitPane(
              pane.id,
              splitOptions.direction,
              undefined,
              splitOptions.placement,
            ) ?? pane.id)
        : pane.id

      workspaceStore.getState().paneActions.setActivePane(targetPaneId)

      try {
        await handleFileOpen(fileDragData.path, false)
        const openedBufferId =
          workspaceStore.getState().paneActions.getActivePane()?.activeBufferId ?? null
        if (openedBufferId) {
          workspaceStore.getState().paneActions.addBufferToPane(targetPaneId, openedBufferId, true)
          workspaceStore.getState().paneActions.activatePaneBuffer(targetPaneId, openedBufferId)
        }
      } catch (error) {
        console.error('Failed to open file from file tree drop:', error)
      } finally {
        delete window.__fileDragData
      }
    },
    [handleFileOpen, pane.id, workspaceStore],
  )

  const handleStageHunk = useCallback(
    async (hunk: GitHunk) => {
      if (!rootFolderPath) return
      try {
        const success = await stageHunk(rootFolderPath, hunk)
        if (success) {
          window.dispatchEvent(new CustomEvent('git-status-changed'))
        }
      } catch (error) {
        console.error('Error staging hunk:', error)
      }
    },
    [rootFolderPath],
  )

  const handleUnstageHunk = useCallback(
    async (hunk: GitHunk) => {
      if (!rootFolderPath) return
      try {
        const success = await unstageHunk(rootFolderPath, hunk)
        if (success) {
          window.dispatchEvent(new CustomEvent('git-status-changed'))
        }
      } catch (error) {
        console.error('Error unstaging hunk:', error)
      }
    },
    [rootFolderPath],
  )

  const handleExternalEditorExit = useCallback(() => {
    if (activeBuffer?.type === 'externalEditor') {
      closeBufferForce(activeBuffer.id)
    }
  }, [activeBuffer, closeBufferForce])

  // Listen for file tree drops on this pane
  useEffect(() => {
    const syncHover = () => {
      const hover = getInternalTabDragHover()
      setInternalHoverZone(hover.paneId === pane.id ? hover.zone : null)
    }

    window.addEventListener('athas-internal-tab-drag-hover', syncHover)
    return () => window.removeEventListener('athas-internal-tab-drag-hover', syncHover)
  }, [pane.id])

  useEffect(() => {
    const handleFileTreeDrop = async (e: CustomEvent) => {
      const fileDragData = window.__fileDragData
      if (!fileDragData) return

      await openFileTreeDropInPane(fileDragData as { path: string; name: string; isDir: boolean }, {
        x: e.detail.x,
        y: e.detail.y,
      })
    }

    window.addEventListener(
      'file-tree-drop-on-pane',
      handleFileTreeDrop as unknown as EventListener,
    )
    return () => {
      window.removeEventListener(
        'file-tree-drop-on-pane',
        handleFileTreeDrop as unknown as EventListener,
      )
    }
  }, [openFileTreeDropInPane])

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()

    const hasTabData =
      e.dataTransfer.types.includes('application/tab-data') || !!getInternalTabDragData()
    const hasFilePath = e.dataTransfer.types.includes('text/plain')
    const hasFileDragData = !!window.__fileDragData

    if (hasTabData || hasFilePath || hasFileDragData || e.dataTransfer.types.includes('Files')) {
      e.dataTransfer.dropEffect = 'move'
      setIsDragOver(true)
      if (hasTabData) {
        setIsTabDragOver(true)
      }
    }
  }, [])

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    const relatedTarget = e.relatedTarget as HTMLElement | null
    const currentTarget = e.currentTarget as HTMLElement
    if (!relatedTarget || !currentTarget.contains(relatedTarget)) {
      setIsDragOver(false)
      setIsTabDragOver(false)
    }
  }, [])

  const handleSplitDrop = useCallback(
    (zone: DropZone, e: React.DragEvent) => {
      setIsDragOver(false)
      setIsTabDragOver(false)

      if (!zone) return

      const tabDataString = e.dataTransfer.getData('application/tab-data')
      const fallbackTabData = getInternalTabDragData()
      if (!tabDataString && !fallbackTabData) return

      let bufferId: string | undefined
      let sourcePaneId: string | undefined
      let source: string | undefined
      let terminalId: string | undefined
      let terminalName: string | undefined
      let initialCommand: string | undefined
      let currentDirectory: string | undefined
      let remoteConnectionId: string | undefined
      try {
        const tabData = tabDataString ? JSON.parse(tabDataString) : fallbackTabData
        bufferId = tabData.bufferId
        sourcePaneId = tabData.paneId
        source = tabData.source
        terminalId = tabData.terminalId
        terminalName = tabData.name
        initialCommand = tabData.initialCommand
        currentDirectory = tabData.currentDirectory
        remoteConnectionId = tabData.remoteConnectionId
      } catch {
        return
      } finally {
        clearInternalTabDragData()
      }

      if (zone === 'center') {
        if (source === 'terminal-panel' && terminalId) {
          const newBufferId = openTerminalBuffer({
            sessionId: terminalId,
            name: terminalName,
            command: initialCommand,
            workingDirectory: currentDirectory,
            remoteConnectionId,
          })
          workspaceStore.getState().paneActions.addBufferToPane(pane.id, newBufferId, true)
          window.dispatchEvent(
            new CustomEvent('terminal-detach-to-buffer', {
              detail: { terminalId },
            }),
          )
        } else if (sourcePaneId && sourcePaneId !== pane.id && bufferId) {
          moveBufferToPaneDropTarget(bufferId, sourcePaneId, { paneId: pane.id, zone: 'center' })
          workspaceStore.getState().paneActions.addBufferToPane(pane.id, bufferId, true)
        } else if (!sourcePaneId && bufferId) {
          ensureBufferInPaneDropTarget(bufferId, { paneId: pane.id, zone: 'center' })
          workspaceStore.getState().paneActions.addBufferToPane(pane.id, bufferId, true)
        }
        return
      }

      // Create the new pane in the workspace store (the authoritative UI source).
      // The legacy usePaneStore utility (getOrCreatePaneDropTarget) wrote to a separate
      // standalone store that the render tree no longer reads — the split would fire but
      // nothing would appear. Using workspaceStore directly fixes the rendering.
      const splitOptions = getPaneSplitDropOptions(zone)
      if (!splitOptions) return // non-center zone always has valid split options
      const newPaneId = workspaceStore
        .getState()
        .paneActions.splitPane(pane.id, splitOptions.direction, undefined, splitOptions.placement)
      if (!newPaneId) return

      // Move the dragged buffer into the newly created pane.
      if (source === 'terminal-panel' && terminalId) {
        const newBufferId = openTerminalBuffer({
          sessionId: terminalId,
          name: terminalName,
          command: initialCommand,
          workingDirectory: currentDirectory,
          remoteConnectionId,
        })
        workspaceStore.getState().paneActions.addBufferToPane(newPaneId, newBufferId, true)
        window.dispatchEvent(
          new CustomEvent('terminal-detach-to-buffer', {
            detail: { terminalId },
          }),
        )
      } else if (sourcePaneId && sourcePaneId !== pane.id && bufferId) {
        // Move from a different source pane into the new split pane
        workspaceStore.getState().paneActions.moveBufferToPane(bufferId, sourcePaneId, newPaneId)
        workspaceStore.getState().paneActions.activatePaneBuffer(newPaneId, bufferId)
      } else if (bufferId) {
        // Move from this pane into the new split pane
        workspaceStore.getState().paneActions.moveBufferToPane(bufferId, pane.id, newPaneId)
        workspaceStore.getState().paneActions.activatePaneBuffer(newPaneId, bufferId)
      }
    },
    [pane.id, openTerminalBuffer, workspaceStore],
  )

  // Handle mouse up for file tree drag (which uses mouse events, not HTML5 drag API)
  const handleMouseUp = useCallback(
    async (event: React.MouseEvent) => {
      const fileDragData = window.__fileDragData
      if (!fileDragData || fileDragData.isDir) {
        return // Only handle file drops, not directory drops
      }

      await openFileTreeDropInPane(fileDragData as { path: string; name: string; isDir: boolean }, {
        x: event.clientX,
        y: event.clientY,
      })
    },
    [openFileTreeDropInPane],
  )

  const handleDrop = useCallback(
    async (e: React.DragEvent) => {
      e.preventDefault()
      e.stopPropagation()
      setIsDragOver(false)
      setIsTabDragOver(false)
      workspaceStore.getState().paneActions.setActivePane(pane.id)

      // Tab drops are handled by SplitDropOverlay — skip here
      if (e.dataTransfer.types.includes('application/tab-data') || getInternalTabDragData()) {
        return
      }

      const droppedPaths = extractDroppedFilePaths(e.dataTransfer)
      if (droppedPaths.length > 0 && handleFileOpen) {
        for (const droppedPath of droppedPaths) {
          await handleFileOpen(droppedPath, false)
        }
        return
      }
    },
    [pane.id, handleFileOpen],
  )

  const renderActiveBuffer = useCallback(
    (buffer: PaneRenderBuffer) => {
      switch (buffer.type) {
        case 'terminal':
          return (
            <TerminalPane
              sessionId={buffer.sessionId}
              bufferId={buffer.id}
              paneId={pane.id}
              initialCommand={buffer.initialCommand}
              workingDirectory={buffer.workingDirectory}
              remoteConnectionId={buffer.remoteConnectionId}
              isActive={isActivePane}
            />
          )

        case 'diff':
          return (
            <DiffPane
              onStageHunk={handleStageHunk}
              onUnstageHunk={handleUnstageHunk}
              isActivePane={isActivePane}
            />
          )

        case 'externalEditor':
          return (
            <ExternalEditorTerminal
              filePath={buffer.path}
              fileName={buffer.name}
              terminalConnectionId={buffer.terminalConnectionId}
              onEditorExit={handleExternalEditorExit}
            />
          )

        case 'crowbarChat':
          // CrowbarChatContent.wsId historically holds the *chat* id — chat
          // buffers are opened with the sidebar chat's id (see chat-tree.tsx).
          return <MarkdownChatView chatId={(buffer as CrowbarChatContent).wsId} />

        case 'branchReview':
          return (
            <BranchReviewPane
              wsId={(buffer as BranchReviewContent).wsId}
              isActivePane={isActivePane}
            />
          )

        default:
          return (
            <EditorPane
              paneId={pane.id}
              bufferId={buffer.id}
              isActiveSurface={isActivePane}
              isPreview={pane.previewBufferId === buffer.id}
              onPromote={() => handlePromote(buffer.id)}
            />
          )
      }
    },
    [
      handleExternalEditorExit,
      handleStageHunk,
      handleUnstageHunk,
      isActivePane,
      pane.id,
      rootFolderPath,
    ],
  )

  return (
    <div
      ref={containerRef}
      data-pane-container
      data-pane-id={pane.id}
      className={`relative flex h-full w-full flex-col overflow-hidden bg-pane-background ${
        isActivePane ? 'ring-1 ring-accent/30' : ''
      } ${isDragOver || internalHoverZone ? 'ring-2 ring-accent' : ''}`}
      onMouseDownCapture={handlePaneMouseDownCapture}
      onClick={handlePaneClick}
      onMouseUp={handleMouseUp}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {(isDragOver || internalHoverZone) && !isTabDragOver && !internalHoverZone && (
        <div className="pointer-events-none absolute inset-0 z-40 bg-accent/10" />
      )}
      <SplitDropOverlay
        visible={isTabDragOver || !!internalHoverZone}
        onDrop={handleSplitDrop}
        activeZoneOverride={internalHoverZone}
      />
      <TabBar
        paneId={pane.id}
        onTabClick={handleTabClick}
        disablePaneActions={pane.id === BOTTOM_PANE_ID}
      />
      <div
        className={cn(
          'relative z-[1] min-h-0 flex-1 overflow-hidden',
          (sidebarPosition === 'left' ? position.atLeft : position.atRight) &&
            'shadow-[-4px_0_8px_rgba(0,0,0,0.12)]',
        )}
        style={paneContentStyle}
      >
        {!activeBuffer && <EmptyEditorState />}

        <Suspense fallback={null}>
          <>
            {/* Keep terminal and webviewer buffers always mounted to preserve
                PTY sessions and embedded webview state. */}
            {paneBuffers
              .filter(
                (b): b is import('../types/pane-content').TerminalContent =>
                  b.type === 'terminal',
              )
              .map((b) => {
                const isActive = b.id === activeBuffer?.id
                return (
                  <div
                    key={b.id}
                    className="absolute inset-0"
                    style={isActive ? undefined : { visibility: 'hidden' }}
                  >
                    <TerminalPane
                      sessionId={b.sessionId}
                      bufferId={b.id}
                      paneId={pane.id}
                      initialCommand={b.initialCommand}
                      workingDirectory={b.workingDirectory}
                      isActive={isActive && isActivePane}
                      isVisible={isActive}
                    />
                  </div>
                )
              })}
            {activeBuffer &&
              activeBuffer.type !== 'terminal' &&
              renderActiveBuffer(activeBuffer)}
          </>
        </Suspense>
      </div>
    </div>
  )
}
