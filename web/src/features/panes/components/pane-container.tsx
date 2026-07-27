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
import { useSidebarOptional } from '@/components/ui/sidebar'
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

import { NewTabView } from './new-tab-view'
import { BOTTOM_PANE_ID } from '../constants/pane'
import {
  useActivePaneId,
  usePaneActions,
  useVisiblePaneCount,
} from '@/features/workspace/stores/hooks/use-pane-store'
import type { PaneGroup } from '../types/pane'
import type { BranchReviewContent, EditorContent } from '../types/pane-content'
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
const BranchReviewPane = lazy(() =>
  import('@/features/git/components/branch-review-pane').then((m) => ({
    default: m.BranchReviewPane,
  })),
)
// Rendered-preview buffers (opened by the breadcrumb eye icon). Each resolves its
// source content from its own buffer's `sourceFilePath` via the store — markdown
// takes the buffer id so it can also retain its scroll across the unmount a tab
// switch causes. Lazy so the markdown parser stays out of the main chunk.
const MarkdownPreview = lazy(() =>
  import('@/features/editor/markdown/markdown-preview').then((m) => ({
    default: m.MarkdownPreview,
  })),
)
const HtmlPreview = lazy(() =>
  import('@/features/editor/components/html/html-preview').then((m) => ({
    default: m.HtmlPreview,
  })),
)
const CsvPreview = lazy(() => import('@/extensions/viewers/csv/csv-preview'))
const AgentChatPane = lazy(() =>
  import('@/features/agent/components/agent-chat-pane').then((m) => ({
    default: m.AgentChatPane,
  })),
)
const EditorPane = lazy(() => import('./editor-pane').then((m) => ({ default: m.EditorPane })))
const DiffPane = lazy(() => import('./diff-pane').then((m) => ({ default: m.DiffPane })))
import { TerminalPane } from './terminal-pane'

interface PaneContainerProps {
  pane: PaneGroup
  position?: PanePosition
}

type EditorBufferShell = Pick<EditorContent, 'id' | 'path' | 'name' | 'type'>
type PaneRenderBuffer =
  | Exclude<import('../types/pane-content').PaneContent, EditorContent>
  | EditorBufferShell

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive pane renderer — resolves pane content to lazily-loaded surfaces and owns split routing; its length is the routing table, not multiple concerns.
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
  // Stable identity: this feeds a memoized drop handler's dep array; an unstable
  // wrapper would defeat that memoization. It only closes over workspaceStore.
  const openTerminalBuffer = useCallback(
    (options?: {
      name?: string
      command?: string
      workingDirectory?: string
      remoteConnectionId?: string
      sessionId?: string
    }): string =>
      workspaceStore.getState().bufferActions.openContent({ type: 'terminal', ...options }),
    [workspaceStore],
  )
  const rootFolderPath = useFileSystemStore.use.rootFolderPath?.()
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
  const isActivePane = pane.id === activePaneId

  // The active-pane ring answers "which of these has focus" — a question that only
  // exists when there is more than one pane on screen. With a single pane it marks the
  // only thing you could possibly be looking at, so it is pure decoration.
  const visiblePaneCount = useVisiblePaneCount()
  // A collapsed sidebar stops shielding the pane from the window frame, so the
  // pane has to square off that edge — see isWindowEdge.
  const sidebarOpen = useSidebarOptional()?.open ?? true
  const paneContentStyle = useMemo(
    () =>
      buildPaneContentStyle(
        position,
        sidebarPosition,
        isActivePane && visiblePaneCount > 1,
        sidebarOpen,
      ),
    [position, sidebarPosition, isActivePane, visiblePaneCount, sidebarOpen],
  )

  const [isDragOver, setIsDragOver] = useState(false)
  const [isTabDragOver, setIsTabDragOver] = useState(false)
  const [internalHoverZone, setInternalHoverZone] = useState<DropZone>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  const rawPaneBuffers = useBuffersByIds(pane.bufferIds)
  const paneBuffers = useMemo((): PaneRenderBuffer[] => {
    return rawPaneBuffers.flatMap((buffer) => {
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

    window.addEventListener('crowbar-internal-tab-drag-hover', syncHover)
    return () => window.removeEventListener('crowbar-internal-tab-drag-hover', syncHover)
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
          // react-doctor-disable-next-line async-await-in-loop -- kept sequential: each open reads the pane's current tab list and appends, so concurrent opens could race on that read-modify-write and land tabs out of drop order. Rare (multi-file drag-drop), not a hot path.
          await handleFileOpen(droppedPath, false)
        }
        return
      }
    },
    [pane.id, handleFileOpen, workspaceStore],
  )

  const renderActiveBuffer = useCallback(
    (buffer: PaneRenderBuffer) => {
      switch (buffer.type) {
        case 'newTab':
          return <NewTabView paneId={pane.id} />

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

        case 'branchReview':
          return (
            <BranchReviewPane
              wsId={(buffer as BranchReviewContent).wsId}
              isActivePane={isActivePane}
            />
          )

        // NOTE: 'agentChat' is intentionally NOT handled here. Like 'terminal', it is
        // hosted by the always-mounted keep-alive block above (visibility:hidden when
        // inactive) so a tab switch never remounts its live PTY — and it is excluded
        // from the active-only Suspense that calls this switch.

        case 'markdownPreview':
          return <MarkdownPreview bufferId={buffer.id} />

        case 'htmlPreview':
          return <HtmlPreview />

        case 'csvPreview':
          return <CsvPreview />

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
      handlePromote,
      handleStageHunk,
      handleUnstageHunk,
      isActivePane,
      pane.id,
      pane.previewBufferId,
    ],
  )

  return (
    <div
      ref={containerRef}
      data-pane-container
      data-pane-id={pane.id}
      // The whole pane is layout chrome: clicking anywhere in it (the tab
      // bar, the editor, empty padding) just marks this pane active as a
      // side effect. Every actually-interactive surface inside — tabs,
      // editor, terminal — is a real focusable, keyboard-operable element
      // that keeps its own role; this wrapper conveys none of its own.
      role="presentation"
      className={`relative flex h-full w-full flex-col overflow-hidden ${
        // Only ring the whole pane for file drags. Tab drags get the inner
        // SplitDropOverlay zone border instead — showing both is a double border.
        isDragOver && !isTabDragOver && !internalHoverZone ? 'ring-2 ring-secondary' : ''
      }`}
      onMouseDownCapture={handlePaneMouseDownCapture}
      onClick={handlePaneClick}
      onMouseUp={handleMouseUp}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {(isDragOver || internalHoverZone) && !isTabDragOver && !internalHoverZone && (
        <div className="pointer-events-none absolute inset-0 z-40 bg-secondary/10" />
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
        // Hook for the drag-time flattening rule in index.css — a rounded,
        // shadowed surface re-rasterised every frame is what makes dragging
        // crawl.
        data-pane-content=""
        className={cn(
          'relative z-[1] min-h-0 flex-1 overflow-hidden bg-pane-background',
          // Casts onto the sidebar, so it only makes sense while there is a
          // sidebar there to catch it.
          sidebarOpen &&
            (sidebarPosition === 'left' ? position.atLeft : position.atRight) &&
            'shadow-[0_3px_8px_rgba(0,0,0,0.24)]',
        )}
        style={paneContentStyle}
      >
        {/* Under the New Tab rules a pane always holds at least one buffer, so
            this should be unreachable. Kept — pointed at the same component — so
            that if a bug ever does strand a pane with no tabs, it shows a usable
            surface instead of a blank rectangle with no way out. */}
        {!activeBuffer && <NewTabView paneId={pane.id} />}

        {/* Keep terminal and webviewer buffers always mounted to preserve PTY
            sessions and embedded webview state. Deliberately OUTSIDE the
            Suspense boundary below: TerminalPane is statically imported (it
            never suspends), so a cold chunk load of whichever lazy pane type
            is active must not transiently unmount these siblings. */}
        {paneBuffers
          .filter(
            (b): b is import('../types/pane-content').TerminalContent => b.type === 'terminal',
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

        {/* Agent chats keep-alive exactly like the terminal block above: each is
            mounted always and only HIDDEN when it isn't the active tab, so its live
            PTY attachment (and its dormant-revive budget) survive a tab switch with
            no remount. AgentChatPane is lazy, so — unlike the statically-imported
            TerminalPane — each buffer needs its OWN Suspense: a cold chunk load must
            resolve to `null` for that one buffer without unmounting its siblings
            (the sibling terminals or other chats). `isVisible` is the ACTIVE TAB
            within this pane; `isActivePane` is whether this pane has focus — they are
            distinct, and the pane threads both. */}
        {paneBuffers
          .filter(
            (b): b is import('../types/pane-content').AgentChatContent => b.type === 'agentChat',
          )
          .map((b) => {
            const isActive = b.id === activeBuffer?.id
            return (
              <div
                key={b.id}
                className="absolute inset-0"
                style={isActive ? undefined : { visibility: 'hidden' }}
              >
                <Suspense fallback={null}>
                  <AgentChatPane
                    chatId={b.chatId}
                    runnerId={b.runnerId}
                    wsId={b.wsId}
                    bufferId={b.id}
                    isActivePane={isActivePane}
                    isVisible={isActive}
                  />
                </Suspense>
              </div>
            )
          })}

        <Suspense fallback={null}>
          {activeBuffer &&
            activeBuffer.type !== 'terminal' &&
            activeBuffer.type !== 'agentChat' &&
            renderActiveBuffer(activeBuffer)}
        </Suspense>
      </div>
    </div>
  )
}

// Prefetch the editor chunk after startup settles: first file-open should not
// pay the network/parse cost, but cold launch must not either (spec P1).
// Exported (rather than fired as a module-scope side effect) so callers control
// WHEN this runs relative to the rest of startup — main.tsx invokes it once
// after the existing init calls. `scheduleIdleTask` falls back to
// `setTimeout(fn, 0)` where `requestIdleCallback` is absent (jsdom, WKWebView),
// so production prefetch happens promptly instead of after a fixed 2s.
