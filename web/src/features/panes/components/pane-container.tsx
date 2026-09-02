import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  useBuffersByIds,
  useBufferActions,
} from '@/features/workspace/stores/hooks/use-buffer-store'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
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
import type {
  BranchReviewContent,
  EditorContent,
  PaneContent,
  TerminalContent,
} from '../types/pane-content'
import {
  ensureBufferInPaneDropTarget,
  moveBufferToPaneDropTarget,
} from '../utils/pane-drop-actions'
import { PANE_DROP_ATTR } from '@/components/sidebar/hooks/use-sidebar-drag'

// Painted straight onto the DOM by `useSidebarDrag`'s own `paintPaneHit` —
// not read here as a prop, since a mounted pane and the sidebar's drag arm
// live in completely different parts of the tree with nothing to prop-drill
// through. Only the Tailwind selector needs to agree with the string
// `paintPaneHit` writes, which is why it is spelled out (`data-[pane-hit]`)
// rather than interpolated from an import — Tailwind's own build has to see
// the literal class name to generate it.
import { type DropZone, SplitDropOverlay } from './split-drop-overlay'
import { PaneSash } from './pane-sash'
import {
  SPLIT_DEFAULT_SIZES,
  SPLIT_MIN_HALF_PX,
  SPLIT_MIN_STACKED_PX,
  usePaneViewPresentation,
} from '@/features/agent/hooks/use-chat-presentation'

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
const CommitDiffPane = lazy(() =>
  import('./commit-diff-pane').then((m) => ({ default: m.CommitDiffPane })),
)
import { TerminalPane } from './terminal-pane'

interface PaneContainerProps {
  pane: PaneGroup
  position?: PanePosition
}

type EditorBufferShell = Pick<EditorContent, 'id' | 'path' | 'name' | 'type' | 'isPreview'>
type PaneRenderBuffer = Exclude<PaneContent, EditorContent> | EditorBufferShell

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive pane renderer — resolves pane content to lazily-loaded surfaces and owns split routing; its length is the routing table, not multiple concerns.
export function PaneContainer({ pane, position = ROOT_PANE_POSITION }: PaneContainerProps) {
  const activePaneId = useActivePaneId()
  const { activateEditorTabInPane, setActivePane } = usePaneActions()
  const bufferActions = useBufferActions()
  const { closeBuffer: closeBufferForce } = bufferActions

  const handlePromote = useCallback(
    (bufferId: string) => {
      bufferActions.promotePreview(bufferId)
    },
    [bufferActions],
  )
  // Needed only to hand AgentChatPane the chat's owning workspace — the chat
  // is no longer a buffer, so there is no per-buffer wsId to read any more.
  const wsId = useWorkspaceStoreContext((s) => s.workspaceId)
  // A freshly opened/dropped buffer must be looked up in the buffer list before
  // it can be added as an editor tab: addEditorTabToPane takes the tab's own
  // EditorTabBase-shaped object (only its `id` is read today, but the object
  // shape is the contract), not a bare id the way the old addBufferToPane did.
  const addExistingTabToPane = useCallback(
    (targetPaneId: string, tabId: string) => {
      const tab = windowPaneStore.getState().buffers.find((b) => b.id === tabId)
      if (tab) windowPaneStore.getState().paneActions.addEditorTabToPane(targetPaneId, tab)
    },
    [],
  )
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
      windowPaneStore.getState().bufferActions.openContent({ type: 'terminal', ...options }),
    [],
  )
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

  const rawPaneBuffers = useBuffersByIds(pane.editorTabIds)
  const paneBuffers = useMemo((): PaneRenderBuffer[] => {
    return rawPaneBuffers.flatMap((buffer) => {
      if (buffer.type === 'editor') {
        return [
          {
            id: buffer.id,
            path: buffer.path,
            name: buffer.name,
            type: buffer.type,
            isPreview: buffer.isPreview,
          } satisfies EditorBufferShell,
        ]
      }
      return [buffer as PaneRenderBuffer]
    })
  }, [rawPaneBuffers])

  const activeBuffer = useMemo(() => {
    if (!pane.activeEditorTabId) return null
    return paneBuffers.find((b) => b.id === pane.activeEditorTabId) || null
  }, [paneBuffers, pane.activeEditorTabId])

  // Spec §7.2's "two views": how the chat view and editor view are arranged
  // when the pane has a chat — side by side (landscape), stacked (portrait),
  // or tabs (too small, or the split toggled off). Geometry only, measured on
  // THIS pane via ResizeObserver — see usePaneViewPresentation. A pane with no
  // chat never reads this: it has no toggle, and its editor view is always
  // the one showing (see the plain fallback branch in the render below).
  const viewsContainerRef = useRef<HTMLDivElement>(null)
  const chatViewRef = useRef<HTMLDivElement>(null)
  const editorViewRef = useRef<HTMLDivElement>(null)
  const [splitSizes, setSplitSizes] = useState<[number, number]>(SPLIT_DEFAULT_SIZES)
  const presentation = usePaneViewPresentation(pane.editorOpen, viewsContainerRef)

  // Spec §7.1/§7.2's split toggle: chat-only vs. chat+editor. A pane with no
  // chat has no toggle to read, so its editor view is always the one showing.
  // This only ever HIDES the editor region (native `hidden` attribute, still
  // mounted) — never unmounts it — per spec §7.2: "Both surfaces stay
  // mounted. `display: none` dormancy is load-bearing — content-visibility
  // melted the CPU." The chat region is never hidden this way: `editorOpen`
  // is "chat-only vs. chat+editor," not "chat vs. editor" — the chat shows in
  // both states.
  //
  // `presentation === 'tabs'` subsumes the old `!pane.editorOpen` check: tabs
  // is reached either because the toggle is off, OR because it is on but the
  // pane is too small to honour it — same downgrade shape as
  // useChatPresentation's own `splitEnabled` gate, just driven by size.
  const editorViewHidden = Boolean(pane.chatId) && presentation === 'tabs'
  // Spec §7.2: "the tab strip moves down between the chat and the editor" in
  // portrait — only when there is a chat to stack the editor view against.
  const isStacked = Boolean(pane.chatId) && presentation === 'stacked'

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
    (tabId: string) => {
      activateEditorTabInPane(pane.id, tabId)
      setActivePane(pane.id)
    },
    [pane.id, activateEditorTabInPane, setActivePane],
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

      // Spec §6.3/§7.2: a file dropped on a pane never gets a pane of its
      // own, regardless of which zone (edge or center) it lands in — it
      // always opens as a tab in the EXISTING pane it was dropped on. Unlike
      // a chat/tab-row drop (handleSplitDrop below), zone is not consulted
      // here at all.
      windowPaneStore.getState().paneActions.setActivePane(pane.id)

      try {
        await handleFileOpen(fileDragData.path, false)
        const openedTabId =
          windowPaneStore.getState().paneActions.getActivePane()?.activeEditorTabId ?? null
        if (openedTabId) {
          addExistingTabToPane(pane.id, openedTabId)
          windowPaneStore.getState().paneActions.activateEditorTabInPane(pane.id, openedTabId)
        }
      } catch (error) {
        console.error('Failed to open file from file tree drop:', error)
      } finally {
        delete window.__fileDragData
      }
    },
    [handleFileOpen, pane.id, addExistingTabToPane],
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
          addExistingTabToPane(pane.id, newBufferId)
          window.dispatchEvent(
            new CustomEvent('terminal-detach-to-buffer', {
              detail: { terminalId },
            }),
          )
        } else if (sourcePaneId && sourcePaneId !== pane.id && bufferId) {
          moveBufferToPaneDropTarget(bufferId, sourcePaneId, { paneId: pane.id, zone: 'center' })
          addExistingTabToPane(pane.id, bufferId)
        } else if (!sourcePaneId && bufferId) {
          ensureBufferInPaneDropTarget(bufferId, { paneId: pane.id, zone: 'center' })
          addExistingTabToPane(pane.id, bufferId)
        }
        return
      }

      // Spec §7.3: "a pane group is a group of chats, never of tabs" (Law 3)
      // — a dragged tab (file/terminal) must never create a new pane/split,
      // on an edge zone any more than on center. This used to open a new
      // pane here (via splitPane + getPaneSplitDropOptions) and move the tab
      // into it; it now lands the tab in THIS pane instead — same action
      // sequence as the split used to run, just targeting the existing pane
      // rather than a freshly created one.
      if (source === 'terminal-panel' && terminalId) {
        const newBufferId = openTerminalBuffer({
          sessionId: terminalId,
          name: terminalName,
          command: initialCommand,
          workingDirectory: currentDirectory,
          remoteConnectionId,
        })
        addExistingTabToPane(pane.id, newBufferId)
        window.dispatchEvent(
          new CustomEvent('terminal-detach-to-buffer', {
            detail: { terminalId },
          }),
        )
      } else if (sourcePaneId && sourcePaneId !== pane.id && bufferId) {
        // Move from a different source pane into this pane.
        windowPaneStore.getState().paneActions.moveEditorTabToPane(bufferId, sourcePaneId, pane.id)
        windowPaneStore.getState().paneActions.activateEditorTabInPane(pane.id, bufferId)
      } else if (bufferId) {
        // Already this pane's own tab (or no source recorded) — nothing to move.
        windowPaneStore.getState().paneActions.activateEditorTabInPane(pane.id, bufferId)
      }
    },
    [pane.id, openTerminalBuffer, addExistingTabToPane],
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
      windowPaneStore.getState().paneActions.setActivePane(pane.id)

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

        case 'commitDiff':
          return <CommitDiffPane sha={buffer.sha} isActivePane={isActivePane} />

        case 'externalEditor':
          return (
            <ExternalEditorTerminal
              // Inherited from EditorTabBase as optional; every real
              // 'externalEditor' tab is constructed with a genuine path.
              filePath={buffer.path ?? ''}
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
              isPreview={buffer.isPreview ?? false}
              onPromote={() => handlePromote(buffer.id)}
            />
          )
      }
    },
    [handleExternalEditorExit, handlePromote, isActivePane, pane.id],
  )

  // Everything pane.editorTabIds holds — files, terminals, branch review,
  // never the chat. Shared verbatim between the two render branches below (a
  // pane with a chat, and a pane without one) so the editor region's actual
  // content is identical either way — only its WRAPPER (and that wrapper's
  // `hidden`/flex-basis) differs by presentation.
  const editorViewInner = (
    <>
      {/* Under the New Tab rules a pane always holds at least one buffer, so
          this should be unreachable. Kept — pointed at the same component — so
          that if a bug ever does strand a pane with no tabs, it shows a usable
          surface instead of a blank rectangle with no way out. */}
      {!activeBuffer && <NewTabView paneId={pane.id} />}

      {/* Keep terminal buffers always mounted to preserve PTY sessions.
          Deliberately OUTSIDE the Suspense boundary below: TerminalPane is
          statically imported (it never suspends), so a cold chunk load of
          whichever lazy pane type is active must not transiently unmount
          these siblings. A terminal is visible only when it is BOTH this
          pane's active editor tab AND the editor view itself isn't hidden
          behind the chat — the two are independent questions (a hidden
          terminal must not be flagged isActive/isVisible just because
          editorViewHidden flips back and forth without a tab switch). */}
      {paneBuffers
        .filter((b): b is TerminalContent => b.type === 'terminal')
        .map((b) => {
          const isActive = b.id === activeBuffer?.id && !editorViewHidden
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

      <Suspense fallback={null}>
        {activeBuffer && activeBuffer.type !== 'terminal' && renderActiveBuffer(activeBuffer)}
      </Suspense>
    </>
  )

  return (
    <div
      ref={containerRef}
      data-pane-container
      data-pane-id={pane.id}
      // The sidebar's own drag arm (`useSidebarDrag`) hit-tests THIS attribute
      // to find which pane a row/chat was dropped onto and at which zone
      // (center/edge) — spec §8.1. Every drop here ADDS; see
      // `performSidebarPaneDrop` (components/sidebar/lib/drop-actions.ts).
      {...{ [PANE_DROP_ATTR]: pane.id }}
      // The whole pane is layout chrome: clicking anywhere in it (the tab
      // bar, the editor, empty padding) just marks this pane active as a
      // side effect. Every actually-interactive surface inside — tabs,
      // editor, terminal — is a real focusable, keyboard-operable element
      // that keeps its own role; this wrapper conveys none of its own.
      role="presentation"
      className={cn(
        'relative flex h-full w-full flex-col overflow-hidden',
        // Only ring the whole pane for file drags. Tab drags get the inner
        // SplitDropOverlay zone border instead — showing both is a double border.
        isDragOver && !isTabDragOver && !internalHoverZone && 'ring-2 ring-secondary',
        // Spec §8.2: "the entry about to take a drop wears the same ring a
        // pane wears" — same token (`ring-secondary`) the active-pane border
        // and the file-drag ring above both already use, so a pane hovered
        // during a sidebar-row drag reads as the SAME kind of "this is the
        // target" as everything else in the app already does.
        'data-[pane-hit]:ring-2 data-[pane-hit]:ring-secondary',
      )}
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
      <div
        // Hook for the drag-time flattening rule in index.css — a rounded,
        // shadowed surface re-rasterised every frame is what makes dragging
        // crawl. Wraps the identity row (TabBar) AND the content below it —
        // ONE shared box, painted, rounded, bordered and shadowed together —
        // not just the content alone. Before this, TabBar sat outside this
        // div, against the unstyled `data-pane-container` shell, and showed
        // the page body's translucent --chrome-bg tint through it: a
        // two-tone "header band over rounded content" look, not the design's
        // single `.pane` surface.
        data-pane-content=""
        className={cn(
          'relative z-[1] flex min-h-0 flex-1 flex-col overflow-hidden bg-pane-background',
          // Casts onto the sidebar, so it only makes sense while there is a
          // sidebar there to catch it.
          sidebarOpen &&
            (sidebarPosition === 'left' ? position.atLeft : position.atRight) &&
            'shadow-[0_3px_8px_rgba(0,0,0,0.24)]',
        )}
        style={paneContentStyle}
      >
        {/* Spec §7.2: in every presentation except 'stacked', the row stays
            put at the top of the pane — see the repositioned copy inside
            viewsContainerRef below for 'stacked'. */}
        {!isStacked && (
          <TabBar
            paneId={pane.id}
            onTabClick={handleTabClick}
            disablePaneActions={pane.id === BOTTOM_PANE_ID}
          />
        )}
        {/* Spec §7.2's "two views": the chat view and the editor view, and how
            they're arranged.

            THIS OUTER DIV, AND THE EDITOR-VIEW DIV INSIDE IT, ARE ALWAYS
            RENDERED — never behind a `pane.chatId ? <A/> : <B/>` branch. That
            was the shape this had until a review caught it: React reconciles
            a ternary's two branches at the SAME tree position, but their
            children differed in count/order (three siblings vs. one), so
            toggling `pane.chatId` reindexed the editor view's own DOM node
            out from under it — React saw a different element at its old
            position and unmounted/remounted it, and everything inside,
            INCLUDING LIVE TERMINAL PTYs, along with it. `pane.chatId` is not
            a hypothetical either — `⌘N` (use-pane-keyboard.ts),
            `openAgentChat`, and a chat drop onto a pane
            (drop-actions.ts's `performSidebarPaneDrop`, Task 22) all call
            `setPaneChat` on a pane that may already be showing something
            else, so this was reachable today, and directly against spec
            §7.2 ("Both surfaces stay mounted") and the terminal keep-alive
            comment below. (The chat-removal.ts caller that used to clear
            `pane.chatId` back to null went with that file — Task 22 — but
            the toggle-BOTH-ways property this fixes is a React
            reconciliation invariant, not a fact about which caller happens
            to exercise it, so it still needs to hold the moment anything
            clears a pane's chat again.)

            The fix: one stable parent, one stable position per child, keyed
            so React's reconciler matches by IDENTITY rather than by index —
            the chat view and the sash are the ones that come and go
            (`pane.chatId && …`), the editor view's own div is unconditional
            and keeps the same key ("editor-view") whether or not it currently
            has a sibling. */}
        <div
          ref={viewsContainerRef}
          className={cn('relative flex min-h-0 flex-1 overflow-hidden', isStacked && 'flex-col')}
        >
          {pane.chatId && (
            // Chat view. Mounted whenever pane.chatId is set — it does not
            // compete with editorTabIds/activeEditorTabId for "which one is
            // active" (a pane holds at most one chat), so isVisible is
            // always true here. NEVER hidden — `editorOpen` means
            // "chat-only vs. chat+editor," not "chat vs. editor": the chat
            // shows in every presentation, including 'tabs'.
            <div
              key="chat-view"
              ref={chatViewRef}
              className={cn(
                'relative min-h-0 min-w-0 overflow-hidden',
                // Tabs: this box IS the pane's content area (the editor sits
                // behind it, `hidden`). Side by side/stacked: it is one half
                // of a real split, sized by splitSizes and left free for the
                // sash to resize (shrink, no grow — same convention
                // agent-chat-pane's own split uses).
                presentation === 'tabs' ? 'h-full w-full flex-1' : 'shrink grow-0',
              )}
              style={presentation === 'tabs' ? undefined : { flexBasis: `${splitSizes[0]}%` }}
            >
              <Suspense fallback={null}>
                {/* `paneId` was `bufferId` and a known, disclosed gap until the
                    final fix wave: AgentChatPane wrote runner-follow repoints
                    and title renames through `bufferActions
                    .repointAgentChatBuffer`/`.renameBuffer`, both of which look
                    an id up in `state.buffers` — and a chat has not been a
                    buffer since Task 1 removed 'agentChat' from PaneContent, so
                    every one of those writes safely no-op'd and runner-follow
                    silently never happened (`/clear` left ChatHead on the old
                    name, and `closePane`'s dormantArrangements push remembered
                    the wrong chat). AgentChatPane now writes
                    `paneActions.setPaneChat(paneId, ...)` — the real write path
                    for what chat a pane holds — and the relabel is gone
                    entirely, since ChatHead reads the live title by chat id. */}
                <AgentChatPane
                  chatId={pane.chatId}
                  runnerId={pane.runnerId ?? ''}
                  wsId={wsId}
                  paneId={pane.id}
                  isActivePane={isActivePane}
                  isVisible
                />
              </Suspense>
            </div>
          )}

          {/* The draggable divider between the two views — side by side or
              stacked only; tabs shows one view at a time, so there is
              nothing to divide. Same imperative pixel sash agent-chat-pane
              uses for its own split, with the identical floor convention:
              the narrower axis (a half's own width side by side, a half's
              own height stacked) gets the smaller floor. */}
          {pane.chatId && presentation !== 'tabs' && (
            <PaneSash
              key="chat-editor-sash"
              direction={presentation === 'stacked' ? 'vertical' : 'horizontal'}
              sizes={splitSizes}
              containerRef={viewsContainerRef}
              firstPaneRef={chatViewRef}
              secondPaneRef={editorViewRef}
              onResizeCommit={setSplitSizes}
              minPx={presentation === 'stacked' ? SPLIT_MIN_STACKED_PX : SPLIT_MIN_HALF_PX}
            />
          )}

          {/* Editor view: everything pane.editorTabIds holds — files,
              terminals, branch review, never the chat. ALWAYS RENDERED
              (never conditionally mounted) — `hidden` only applies the
              native `hidden` attribute (display:none via the UA
              stylesheet, not a Tailwind class, so it needs no compiled CSS
              to take effect) in 'tabs' presentation (spec §7.1/§7.2: the
              split is off, or the pane is too small to honour it, or there
              is no chat at all). Everything inside — including the terminal
              keep-alive block — stays mounted across every presentation
              change AND across pane.chatId itself changing: this div keeps
              the same key ("editor-view") and the same position among its
              siblings regardless of whether the chat-view/sash siblings
              above exist, so React never sees it as a different element —
              only its `hidden` attribute and its sizing change, and nothing
              inside it ever unmounts/remounts. See the block comment at the
              top of this section for the bug this fixes. */}
          {/* Spec §7.2: "the tab strip moves down between the chat and the
              editor" in 'stacked' presentation — the repositioned copy of
              the row that renders at the top of the pane in every other
              presentation (see the `!isStacked` guard above). `TabBar`
              fuses the split toggle, the chat head, AND the tab strip into
              one row (spec §7.1) with no internal seam to split just the
              strip out of — so this moves the WHOLE row down, split toggle
              and chat name included, rather than leaving the tab strip
              pinned at the top on its own. Splitting TabBar into separable
              head/strip pieces so the chat name can stay fixed in the head
              while only the strip travels is a distinct change with its own
              blast radius (`tab-bar.tsx`), out of this task's declared file
              scope — flagged here rather than silently left as the prior
              gap was. */}
          {isStacked && (
            <TabBar
              key="tab-bar-stacked"
              paneId={pane.id}
              onTabClick={handleTabClick}
              disablePaneActions={pane.id === BOTTOM_PANE_ID}
            />
          )}
          <div
            key="editor-view"
            ref={editorViewRef}
            hidden={editorViewHidden}
            className={cn(
              'relative min-h-0 overflow-hidden',
              Boolean(pane.chatId) && presentation !== 'tabs' ? 'shrink grow-0' : 'w-full flex-1',
            )}
            style={
              Boolean(pane.chatId) && presentation !== 'tabs'
                ? { flexBasis: `${splitSizes[1]}%` }
                : undefined
            }
          >
            {editorViewInner}
          </div>
        </div>
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
