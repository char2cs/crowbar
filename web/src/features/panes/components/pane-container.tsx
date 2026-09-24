import type { CSSProperties } from 'react'
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  useBuffersByIds,
  useBufferActions,
} from '@/features/workspace/stores/hooks/use-buffer-store'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
  WorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import {
  useChatWorkspaceId,
  useChatWorkspaceHint,
} from '@/features/panes/hooks/use-chat-workspace-id'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useSettingsStore } from '@/features/settings/store'
import { buildInnerViewStyle, buildPaneContentStyle } from '../utils/pane-border'
import { useSidebarOptional } from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'
import { ROOT_PANE_POSITION, type PanePosition } from '../types/pane'
import TabBar from '@/features/tabs/components/tab-bar'
import { ChatOnlyPaneHeader } from '@/features/tabs/components/chat-only-pane-header'
import { ChatColumnHeader } from '@/features/tabs/components/chat-column-header'
import { extractDroppedFilePaths } from '@/features/file-system/utils/file-system-dropped-paths'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import {
  clearInternalTabDragData,
  getInternalTabDragData,
  getInternalTabDragHover,
  resolveDropTarget,
} from '@/features/tabs/utils/internal-tab-drag'

import { NewTabView } from './new-tab-view'
import { BOTTOM_PANE_ID } from '../constants/pane'
import {
  useIsActivePane,
  usePaneActions,
  useVisiblePaneCount,
} from '@/features/workspace/stores/hooks/use-pane-store'
import type { PaneGroup } from '../types/pane'
import type {
  BranchReviewContent,
  CommitDiffContent,
  EditorContent,
  PaneContent,
  TerminalContent,
} from '../types/pane-content'
import { ensureBufferInPaneDropTarget } from '../utils/pane-drop-actions'
import { clearEditorPortalEntry, setEditorPortalEntry } from '../lib/editor-portal-registry'
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
const CommitDiffPane = lazy(() =>
  import('./commit-diff-pane').then((m) => ({ default: m.CommitDiffPane })),
)
import { TerminalPane } from './terminal-pane'

interface PaneContainerProps {
  pane: PaneGroup
  position?: PanePosition
  /** Whether this pane's VIEW is the one on screen — see PaneNodeRenderer.
   *  A parked view's panes stay mounted (so nothing they hold is torn down)
   *  but must do no work: every surface below is handed `isActive`/`isVisible`
   *  false, which is the same dormant state a background TAB inside a pane
   *  already runs in, so no surface needs a second notion of "hidden". */
  showing?: boolean
}

type EditorBufferShell = Pick<EditorContent, 'id' | 'path' | 'name' | 'type' | 'isPreview'>
type PaneRenderBuffer = Exclude<PaneContent, EditorContent> | EditorBufferShell

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive pane renderer — resolves pane content to lazily-loaded surfaces and owns split routing; its length is the routing table, not multiple concerns.
export function PaneContainer({
  pane,
  position = ROOT_PANE_POSITION,
  showing = true,
}: PaneContainerProps) {
  // A BOOLEAN, not the id: subscribing to `activePaneId` itself re-rendered
  // EVERY pane in the window (and its whole chat/editor subtree) whenever focus
  // moved between any two of them, where only the two that actually changed
  // have anything to redraw.
  const isActiveInStore = useIsActivePane(pane.id)
  const { activateEditorTabInPane, setActivePane } = usePaneActions()
  const bufferActions = useBufferActions()
  const { closeBuffer: closeBufferForce } = bufferActions

  // THE CHAT'S OWN WORKSPACE, not the one that happens to be on screen.
  //
  // Panes are window-level (Task 26) and a drop can put ANY workspace's chat
  // in one, but a chat's own state and every chat-scoped URL are still
  // workspace-keyed. Reading the ambient `WorkspaceStoreContext` here — the
  // `WorkspaceView` that happens to be rendering this pane — is what made a
  // chat from another workspace render permanently blank (its id is not in
  // the on-screen workspace's `agentChats.chats`, so nothing ever attaches),
  // and is the gap `openChatIntoPane`'s active-workspace refusal stood in for.
  // `useChatWorkspaceId` resolves the real owner (features/panes/lib/
  // pane-chat-workspace.ts); the ambient id remains the fallback for a chat
  // nothing can name a workspace for yet. `useChatWorkspaceHint` is what
  // makes that resolution work on the FIRST render a pane holds a chat
  // nothing has mounted yet — without it, resolution could only come from
  // the registry, populated by WorkspaceHost's reconcile effect AFTER the
  // click that made this pane active, not synchronously with it: a flash of
  // the wrong ambient content on every such click, self-correcting a frame
  // or two later — caught live.
  const ambientWsId = useWorkspaceStoreContext((s) => s.workspaceId)
  const ambientStore = useWorkspaceStore()
  const chatWsHint = useChatWorkspaceHint(pane.chatId)
  const chatWsId = useChatWorkspaceId(pane.chatId, chatWsHint)
  const wsId = chatWsId ?? ambientWsId
  // The STORE half of the same answer. `getWorkspaceStore` never mints one —
  // a workspace `WorkspaceHost` did not mount has no store to read and would
  // leak a permanent, unmanaged one if this created it — so a chat whose
  // owner has been evicted falls back to the ambient store, which (chat lists
  // being repo-scoped) still knows every chat of its own repo.
  const chatStore = (chatWsId && getWorkspaceStore(chatWsId)) || ambientStore
  // A freshly opened/dropped buffer must be looked up in the buffer list before
  // it can be added as an editor tab: addEditorTabToPane takes the tab's own
  // EditorTabBase-shaped object (only its `id` is read today, but the object
  // shape is the contract), not a bare id the way the old addBufferToPane did.
  const addExistingTabToPane = useCallback((targetPaneId: string, tabId: string) => {
    const tab = windowPaneStore.getState().buffers.find((b) => b.id === tabId)
    if (tab) windowPaneStore.getState().paneActions.addEditorTabToPane(targetPaneId, tab)
  }, [])
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
  // A pane in a PARKED view is never the active one, whatever `activePaneId`
  // says: `activePaneId` names one pane for the whole window, and a view that
  // is off screen has no claim on it. Folded in here rather than at each of
  // the six surfaces below so no surface can be given a live `isActive` for a
  // view nobody is looking at.
  const isActivePane = showing && isActiveInStore

  // The active-pane ring answers "which of these has focus" — a question that only
  // exists when there is more than one pane on screen. With a single pane it marks the
  // only thing you could possibly be looking at, so it is pure decoration.
  const visiblePaneCount = useVisiblePaneCount()
  // A collapsed sidebar stops shielding the pane from the window frame, so the
  // pane has to square off that edge — see isWindowEdge.
  const sidebarOpen = useSidebarOptional()?.open ?? true
  const showActiveBorder = isActivePane && visiblePaneCount > 1
  // `false`, ALWAYS — the shared box's own border never changes colour.
  //
  // It used to carry the accent directly, faded by `transition-colors` (see
  // the class on that element below). But this box is huge, rounded, and
  // painted in translucent `--chrome-bg` over the window's real vibrancy, so
  // WebKit re-blends the whole rounded surface on every frame a colour on it
  // interpolates: one focus click bought a ~150ms train of 17-26ms frames.
  // Measured live, three panes tiled in one view, 12 focus clicks: 85fps with
  // the border-colour fade, 116fps with it suppressed, and identical React
  // work either way. Neither `contain: paint` (82.9fps) nor forcing a
  // compositing layer (67.8fps) helped — the repaint is inherent to animating
  // a colour on this surface, so the accent had to come off it.
  //
  // `paneAccentStyle` below draws the accent instead, as an empty overlay
  // fading its OPACITY, which the compositor handles without repainting
  // anything underneath. The rest of the box (the neutral border, the radii,
  // the margins) is unchanged, and `transition-colors` stays for the
  // background, which still has a theme swap to fade.
  const paneContentStyle = useMemo(
    () => buildPaneContentStyle(position, sidebarPosition, false, sidebarOpen),
    [position, sidebarPosition, sidebarOpen],
  )
  // THE ACCENT RING — the very same border/radius `buildPaneContentStyle`
  // would have put on the box itself (asked for with `showActiveBorder` true,
  // so the per-edge `none` rules and per-corner radii stay computed in exactly
  // one place), lifted onto a childless sibling that covers the box's border
  // box. Its margins become insets: the pane root is `relative`, and the box
  // is inset from it by exactly those margins, so the overlay's own 2px border
  // lands precisely on top of the neutral one it hides.
  const paneAccentStyle = useMemo<CSSProperties>(() => {
    const accent = buildPaneContentStyle(position, sidebarPosition, true, sidebarOpen)
    return {
      position: 'absolute',
      left: accent.marginLeft,
      top: accent.marginTop,
      right: accent.marginRight,
      bottom: accent.marginBottom,
      borderTop: accent.borderTop,
      borderLeft: accent.borderLeft,
      borderRight: accent.borderRight,
      borderBottom: accent.borderBottom,
      borderTopLeftRadius: accent.borderTopLeftRadius,
      borderTopRightRadius: accent.borderTopRightRadius,
      borderBottomLeftRadius: accent.borderBottomLeftRadius,
      borderBottomRightRadius: accent.borderBottomRightRadius,
    }
  }, [position, sidebarPosition, sidebarOpen])

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
  // Chats/pane redesign: a pane with a chat and NO editor tabs at all has no
  // IDE sector to speak of — "if the user has no tabs opened, then just the
  // chat view should be shown," full stop, regardless of the split toggle or
  // available room. This overrides `presentation` below rather than feeding
  // into `usePaneViewPresentation` itself: that hook answers a pure geometry
  // question ("does this pane have room for a split"), and tab COUNT is a
  // separate, independent reason to collapse to chat-only.
  const hasEditorTabs = pane.editorTabIds.length > 0
  const chatFillsPane = Boolean(pane.chatId) && !hasEditorTabs

  // Chats/pane redesign: in the collapsed ('tabs') presentation WITH real
  // editor tabs, the chat is a real, selectable surface — "just another tab"
  // — rather than the fixed winner it is everywhere else. `pane.chatSelected`
  // is the SAME fact TabBar's own ChatTabItem reads (see showChatTab below);
  // read defensively (`!== false`) for a pane restored from a layout saved
  // before this field existed (PaneGroup.chatSelected's own doc).
  const chatSelectedInTabsMode = pane.chatSelected !== false
  const showChatTab = Boolean(pane.chatId) && hasEditorTabs && presentation === 'tabs'
  // Chats/pane redesign bug fix: side by side and stacked both show the chat
  // and the editor SIMULTANEOUSLY, as two separate boxes — the IDE sector's
  // own tab strip belongs confined to the editor's own column/row, never
  // spanning over the chat's too. `showTopLevelHeader` below is the ONLY
  // state where one header legitimately spans the whole pane: a single
  // surface (chat OR one editor tab) filling 100% of it.
  const chatVisibleAlongsideEditor =
    Boolean(pane.chatId) && !chatFillsPane && presentation !== 'tabs'
  const showTopLevelHeader = !pane.chatId || chatFillsPane || presentation === 'tabs'
  const isBottomPane = pane.id === BOTTOM_PANE_ID

  // `presentation === 'tabs'` subsumes the old `!pane.editorOpen` check: tabs
  // is reached either because the toggle is off, OR because it is on but the
  // pane is too small to honour it — same downgrade shape as
  // useChatPresentation's own `splitEnabled` gate, just driven by size.
  // `chatFillsPane` is a THIRD reason, driven by tab count instead of size or
  // preference — see its own doc above. Editor view hides in EITHER tabs
  // case UNLESS a real tab is the one selected (`showChatTab &&
  // !chatSelectedInTabsMode`), in which case it's the chat that hides
  // instead (`chatViewHidden` below).
  const editorViewHidden =
    Boolean(pane.chatId) &&
    (chatFillsPane || (presentation === 'tabs' && (!showChatTab || chatSelectedInTabsMode)))

  // Editor-portal target: see editor-host-registry.tsx's own doc for why the
  // actual EditorPane/Monaco widget lives OUTSIDE this component entirely.
  // This pane publishes only WHERE it wants that widget portaled and what it
  // should currently show — never renders it directly.
  const editorPortalTargetRef = useRef<HTMLDivElement>(null)
  const hasOpenEditorBuffer = paneBuffers.some((b) => b.type === 'editor')
  const isEditorTabActive = activeBuffer?.type === 'editor' && !editorViewHidden
  const activeEditorBuffer = activeBuffer?.type === 'editor' ? activeBuffer : null
  useEffect(() => {
    const node = editorPortalTargetRef.current
    if (!hasOpenEditorBuffer || !node) {
      clearEditorPortalEntry(pane.id)
      return
    }
    setEditorPortalEntry(pane.id, {
      node,
      activeEditorBufferId: activeEditorBuffer?.id ?? null,
      isPreview: activeEditorBuffer?.isPreview ?? false,
      isActiveSurface: isEditorTabActive && isActivePane,
    })
    return () => clearEditorPortalEntry(pane.id)
  }, [
    pane.id,
    hasOpenEditorBuffer,
    activeEditorBuffer?.id,
    activeEditorBuffer?.isPreview,
    isEditorTabActive,
    isActivePane,
  ])
  // The chat is "NEVER hidden" everywhere else in this file (side by side,
  // stacked, chatFillsPane) — per spec §7.2, `editorOpen` alone never hides
  // it. This is the one exception: collapsed presentation, real tabs to
  // switch to, and a real tab (not the chat) currently selected.
  const chatViewHidden = showChatTab && !chatSelectedInTabsMode
  // Spec §7.2: "the tab strip moves down between the chat and the editor" in
  // portrait — only when there is a chat to stack the editor view against,
  // and only when there IS an editor view worth stacking against at all.
  const isStacked = Boolean(pane.chatId) && presentation === 'stacked' && !chatFillsPane

  // The chat sits next to wherever the sidebar is: a sidebar on the right
  // means the chat renders on the right too, so the two never end up on
  // opposite edges of the window. Stacked has no left/right sidebar concept
  // at all, so it always keeps the chat on top regardless of sidebar side.
  const chatIsFirst = presentation === 'stacked' || sidebarPosition !== 'right'

  // Which of the editor-view's own edges is genuinely internal — facing the
  // chat's own box, never a window edge whatever `paneContentStyle` says
  // about it. Only meaningful while chatVisibleAlongsideEditor (side by
  // side/stacked with both boxes on screen); unused otherwise.
  const editorFacingChatEdge = presentation === 'stacked' ? 'top' : chatIsFirst ? 'left' : 'right'
  const editorInnerViewStyle = useMemo(
    () => (chatVisibleAlongsideEditor ? buildInnerViewStyle(editorFacingChatEdge) : undefined),
    [chatVisibleAlongsideEditor, editorFacingChatEdge],
  )

  const handlePaneClick = useCallback(() => {
    if (!isActivePane) {
      setActivePane(pane.id)
    }
  }, [isActivePane, pane.id, setActivePane])

  // A REAL native listener, not a React `onMouseDownCapture` prop — deliberately.
  // EditorPane/Monaco (editor-host-registry.tsx) is portaled in from
  // EditorHostRegistry, a React SIBLING of the whole pane tree, not a
  // descendant of PaneContainer. Its DOM node still lands inside this pane's
  // own subtree (the portal TARGET div this component publishes below), but
  // React's synthetic dispatch collects ancestor handlers by walking the
  // FIBER tree, not the DOM tree — a portal's bubble/capture path runs
  // through its React parent (EditorHostSlot), never through PaneContainer.
  // A React `onMouseDownCapture` prop here (what this used to be) therefore
  // NEVER fired for a click landing on the real Monaco widget, no matter how
  // many explicit `setActivePane` calls got added to individual handlers
  // elsewhere — none of them sit on the click's actual path. Attaching
  // straight to the DOM node in capture phase only cares about real DOM
  // containment, so it catches the portaled content too, same as it catches
  // everything else already flowing through here.
  useEffect(() => {
    const node = containerRef.current
    if (!node) return
    const onMouseDownCapture = (e: MouseEvent) => {
      const target = e.target as HTMLElement
      // Monaco's own real text-input surface ('inputarea' —
      // textAreaEditContext.js) and xterm's own real helper textarea must
      // still activate the pane even though they're literal <textarea>
      // elements: they're the actual typing surface, not a decorative
      // control.
      const isEditorTextarea = target.classList?.contains('inputarea')
      const isTerminalTextarea = target.classList?.contains('xterm-helper-textarea')
      // Checked against the mousedown's own TARGET only, never `.closest()`
      // — chat message content, Monaco's toolbar/find-bar and Plate's
      // toolbar all nest real buttons throughout their content, and a
      // `.closest()` walk swallowed activation for a mousedown anywhere
      // inside one of those ancestors, not just a direct hit on the control
      // itself.
      const isDirectInteractiveHit = target.matches?.(
        "button, input, textarea, [role='button'], [role='menu']",
      )
      if (!isEditorTextarea && !isTerminalTextarea && isDirectInteractiveHit) {
        return
      }

      // Read fresh rather than closing over `isActivePane`/`showing`: a
      // parked view's pane never receives a real user gesture (it is off
      // screen), so `activePaneId` alone is the whole answer, and reading it
      // live means this listener never needs to be torn down and re-added
      // just because focus moved.
      if (windowPaneStore.getState().activePaneId !== pane.id) {
        windowPaneStore.getState().paneActions.setActivePane(pane.id)
      }
    }
    node.addEventListener('mousedown', onMouseDownCapture, true)
    return () => node.removeEventListener('mousedown', onMouseDownCapture, true)
  }, [pane.id])

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
      // The external process is already gone, so this buffer must be torn down
      // regardless of how many panes still list it (an externalEditor buffer is
      // shareable across a split, same as an ordinary editor — see
      // getShareableSplitBufferId). closeBuffer only tears a buffer down once NO
      // pane references the id any more, reading any remaining membership as a
      // SIBLING still showing a live split — which this is not. Strip every
      // pane's membership first.
      const bufferId = activeBuffer.id
      const state = windowPaneStore.getState()
      for (const p of Object.values(state.panes)) {
        if (p.editorTabIds.includes(bufferId)) {
          state.paneActions.removeEditorTabFromPane(p.id, bufferId)
        }
      }
      closeBufferForce(bufferId)
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

      // A tab may never cross a pane boundary via drag — it is only ever
      // reorderable within its own pane's own tab bar (see use-tab-drag.ts's
      // own doc). `sourcePaneId` naming a DIFFERENT pane than this one is
      // exactly that forbidden gesture, so it is rejected outright below in
      // both zones — never moved, never opened here — regardless of what a
      // payload happens to carry.
      const isCrossPaneTabDrop = Boolean(
        sourcePaneId && sourcePaneId !== pane.id && bufferId && source !== 'terminal-panel',
      )

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
        } else if (!isCrossPaneTabDrop && bufferId) {
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
      } else if (!isCrossPaneTabDrop && bufferId) {
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

  const handleTauriFileDrop = useCallback(
    async (paths: string[]) => {
      if (paths.length === 0 || !handleFileOpen) return
      windowPaneStore.getState().paneActions.setActivePane(pane.id)
      for (const droppedPath of paths) {
        // react-doctor-disable-next-line async-await-in-loop -- kept sequential: each open reads the pane's current tab list and appends, so concurrent opens could race on that read-modify-write and land tabs out of drop order. Rare (multi-file drag-drop), not a hot path.
        await handleFileOpen(droppedPath, false)
      }
    },
    [pane.id, handleFileOpen],
  )
  useTauriFileDrop(containerRef, handleTauriFileDrop)

  const renderActiveBuffer = useCallback(
    (buffer: PaneRenderBuffer) => {
      switch (buffer.type) {
        case 'terminal':
          return (
            <TerminalPane
              sessionId={buffer.sessionId}
              bufferId={buffer.id}
              paneId={pane.id}
              workspaceId={buffer.workspaceId}
              initialCommand={buffer.initialCommand}
              workingDirectory={buffer.workingDirectory}
              remoteConnectionId={buffer.remoteConnectionId}
              isActive={isActivePane}
            />
          )

        case 'commitDiff':
          // wsId is the buffer's OWN workspace (CommitDiffContent.wsId), not
          // the ambient ws above — see review-diff-tab.tsx's `wsId` prop doc
          // for why a hidden keep-alive copy must not re-derive it from
          // context.
          return (
            <CommitDiffPane
              sha={buffer.sha}
              wsId={(buffer as CommitDiffContent).wsId}
              isActivePane={isActivePane}
            />
          )

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
          // 'editor' buffers never reach here — see the editor-portal target
          // below, rendered unconditionally alongside the terminal keep-alive
          // block for the same reason terminals are: EditorHostRegistry (a
          // sibling of the whole pane tree, not a descendant of it) owns the
          // actual EditorPane/Monaco widget so it survives this pane's own
          // subtree being torn down and rebuilt by a tab switch, a split, or
          // fullscreen.
          return null
      }
    },
    [handleExternalEditorExit, isActivePane, pane.id],
  )

  // Everything pane.editorTabIds holds — files, terminals, branch review,
  // never the chat. Shared verbatim whether or not the pane also holds a chat,
  // so the editor region's actual content is identical either way — only its
  // WRAPPER (and that wrapper's `hidden`/flex-basis) differs by presentation.
  const editorViewInner = (
    <>
      {/* NOT a second, ordinary kind of pane — a FALLBACK for a pane holding
          nothing, which "should only appear when NO VIEW is opened". An
          emptied pane in a split now leaves the layout outright
          (`dropEmptiedPanes`, pane-slice.ts), so the only pane that reaches
          this with no chat either is the last one in the window, and TabBar
          draws that one with no chrome to name or close it. Deliberately inert
          (just the wordmark) — a pane with nothing in it must never be a
          screen a user can act from; see new-tab-view.tsx. */}
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
                workspaceId={b.workspaceId}
                initialCommand={b.initialCommand}
                workingDirectory={b.workingDirectory}
                isActive={isActive && isActivePane}
                // A terminal in a PARKED view is not visible however this
                // pane has its own tabs arranged — `showing` is the outer of
                // the two questions, and xterm gates its render loop on
                // exactly this flag. Without it a hidden view's terminal
                // would keep drawing into a `display: none` subtree.
                isVisible={isActive && showing}
              />
            </div>
          )
        })}

      {/* Editor-portal target — see editor-host-registry.tsx's own doc. The
          actual EditorPane/Monaco widget is portaled in here from OUTSIDE
          this pane's own subtree, so it survives this component (and this
          div) being torn down and rebuilt by a tab switch, a split, or
          fullscreen: only a NEW target node gets registered, the SAME live
          widget just gets reparented into it. Rendered whenever the pane has
          any open editor buffer, not only while one is the active tab — same
          "always mounted, visibility toggled" shape as the terminal block
          above, for the same reason. */}
      {hasOpenEditorBuffer && (
        <div
          ref={editorPortalTargetRef}
          className="absolute inset-0"
          style={isEditorTabActive ? undefined : { visibility: 'hidden' }}
        />
      )}

      <Suspense fallback={null}>
        {activeBuffer &&
          activeBuffer.type !== 'terminal' &&
          activeBuffer.type !== 'editor' &&
          renderActiveBuffer(activeBuffer)}
      </Suspense>
    </>
  )

  // Chat view. Mounted whenever pane.chatId is set — it does not compete
  // with editorTabIds/activeEditorTabId for "which one is active" (a pane
  // holds at most one chat), so isVisible is always true here. NEVER
  // hidden — `editorOpen` means "chat-only vs. chat+editor," not "chat vs.
  // editor": the chat shows in every presentation, including 'tabs'.
  //
  // Computed here (rather than inline in the return below) so the chat
  // view, the sash and the editor view can be placed in either DOM order —
  // chat first (sidebar on the left, or stacked) or editor first (sidebar
  // on the right) — from a single ternary, instead of duplicating each
  // block. Each still carries its own stable key, so reordering them is
  // not a remount: React's reconciler matches by key, not by position.
  const chatViewNode = pane.chatId && (
    <div
      key="chat-view"
      ref={chatViewRef}
      data-chat-view=""
      hidden={chatViewHidden}
      className={cn(
        // No fill of its own — the shared `data-pane-content` box (above)
        // already paints `bg-chrome-bg`; painting it AGAIN here would
        // stack two translucent layers and read visibly more opaque than
        // either alone.
        'relative flex min-h-0 min-w-0 flex-col overflow-hidden',
        // Tabs, or no tabs at all: this box IS the pane's content area
        // (the editor sits behind it, `hidden`). Side by side/stacked: it
        // is one half of a real split, sized by splitSizes and left free
        // for the sash to resize (shrink, no grow — same convention
        // agent-chat-pane's own split uses).
        presentation === 'tabs' || chatFillsPane ? 'h-full w-full flex-1' : 'shrink grow-0',
      )}
      style={
        presentation === 'tabs' || chatFillsPane
          ? undefined
          : // splitSizes is always [chatPct, editorPct] regardless of which
            // side the chat visually renders on — the SASH (below) is what
            // maps that fixed pair onto whichever pane is visually first.
            { flexBasis: `${splitSizes[0]}%` }
      }
    >
      {/* The chat surface reads its workspace store off CONTEXT
          (`useWorkspaceStore`), so handing it the right `wsId` is only
          half the answer — it has to READ from that workspace's own
          store too, or a chat from another repo is simply never found.
          Re-provided here, around the chat view alone: everything else
          in this pane (editor tabs, terminals) genuinely belongs to
          the workspace whose view is on screen, and must keep it.
          ChatBranchHeader (the chat's own identity header, replacing
          ChatHead's old spot in tab-bar.tsx) sits inside the SAME
          provider so it resolves the chat's title off its OWN
          workspace too, not whichever one is ambient.

          Three ways this box's own header renders, matching
          `showTopLevelHeader`/`chatVisibleAlongsideEditor` above:
            - chatFillsPane: absent — ChatOnlyPaneHeader (top of the
              whole pane) already shows it.
            - side by side/stacked: ChatColumnHeader — this box IS a
              real column/row next to the editor's, so it gets the
              full window-chrome treatment (drag region, traffic
              lights) — it may be the pane's actual top-left corner.
            - collapsed ('tabs'), chat currently selected: the small,
              chrome-less ChatBranchHeader — TabBar's OWN full-pane
              row (above, outside this box) already owns the window
              chrome in this state. */}
      <WorkspaceStoreContext.Provider value={chatStore}>
        {chatVisibleAlongsideEditor && (
          <ChatColumnHeader chatId={pane.chatId} wsId={chatWsId} isBottomPane={isBottomPane} />
        )}
        <div className="relative min-h-0 flex-1 overflow-hidden">
          <Suspense fallback={null}>
            <AgentChatPane
              chatId={pane.chatId}
              runnerId={pane.runnerId ?? ''}
              wsId={wsId}
              paneId={pane.id}
              isActivePane={isActivePane}
              // ChatOnlyPaneHeader (chatFillsPane) and ChatColumnHeader
              // (chatVisibleAlongsideEditor) both render as an overlay
              // chat-blur PaneTopRow — a REAL, clickable row floating
              // above this pane with no flex space of its own. The
              // collapsed 'tabs' presentation's small in-flow
              // ChatBranchHeader already reserves its own space, so
              // there is nothing for this pane to additionally clear.
              belowOverlayHeader={chatFillsPane || chatVisibleAlongsideEditor}
              // Was hard-coded true: a pane holds at most one chat, so
              // within the pane the chat view is always the one showing.
              // With views that is no longer the whole question — the
              // pane itself can be in an arrangement that is off screen,
              // and now (chats/pane redesign) a selected EDITOR tab can
              // cover the chat within an otherwise-visible pane too
              // (`chatViewHidden`). It matters beyond appearances: the
              // dormant-chat revive fires on `isVisible`, so a parked or
              // covered chat left claiming to be visible would spawn a
              // vendor CLI for a chat nobody is looking at.
              isVisible={showing && !chatViewHidden}
            />
          </Suspense>
        </div>
      </WorkspaceStoreContext.Provider>
    </div>
  )

  // The draggable divider between the two views — side by side or stacked
  // only; tabs shows one view at a time, so there is nothing to divide.
  // Same imperative pixel sash agent-chat-pane uses for its own split,
  // with the identical floor convention: the narrower axis (a half's own
  // width side by side, a half's own height stacked) gets the smaller
  // floor. `firstPaneRef`/`secondPaneRef` follow `chatIsFirst` — the sash's
  // own pixel math assumes "first" means "visually first" (left/top),
  // whichever pane that actually is.
  const sashNode = pane.chatId && presentation !== 'tabs' && !chatFillsPane && (
    <PaneSash
      key="chat-editor-sash"
      direction={presentation === 'stacked' ? 'vertical' : 'horizontal'}
      sizes={chatIsFirst ? splitSizes : [splitSizes[1], splitSizes[0]]}
      containerRef={viewsContainerRef}
      firstPaneRef={chatIsFirst ? chatViewRef : editorViewRef}
      secondPaneRef={chatIsFirst ? editorViewRef : chatViewRef}
      onResizeCommit={(sizes) => setSplitSizes(chatIsFirst ? sizes : [sizes[1], sizes[0]])}
      minPx={presentation === 'stacked' ? SPLIT_MIN_STACKED_PX : SPLIT_MIN_HALF_PX}
      // No fill of its own needed — the shared `data-pane-content`
      // box's own `bg-chrome-bg` already shows through here (this
      // sash paints nothing at rest), so it reads as a continuation
      // of the chat's own tint rather than a seam of nothing.
    />
  )

  // Editor view: everything pane.editorTabIds holds — files, terminals,
  // branch review, never the chat. ALWAYS RENDERED (never conditionally
  // mounted) — `hidden` only applies the native `hidden` attribute
  // (display:none via the UA stylesheet, not a Tailwind class, so it needs
  // no compiled CSS to take effect) in 'tabs' presentation (spec §7.1/
  // §7.2: the split is off, or the pane is too small to honour it, or
  // there is no chat at all). Everything inside — including the terminal
  // keep-alive block — stays mounted across every presentation change AND
  // across pane.chatId itself changing: this node keeps the same key
  // ("editor-view") regardless of whether the chat-view/sash siblings
  // exist OR which side of them it renders on, so React never sees it as
  // a different element — only its `hidden` attribute and its sizing
  // change, and nothing inside it ever unmounts/remounts. See the chat
  // view's own block comment above for the bug this fixes.
  //
  // Chats/pane redesign bug fix: TabBar is the IDE SECTOR's own header, so
  // whenever the chat is ALSO visible at the same time (side by side or
  // stacked — `chatVisibleAlongsideEditor`), it belongs confined to the
  // editor view's own box, not spanning over the chat's column/row too
  // (that was the actual bug: tabs visibly sitting above the chat). One
  // placement covers both side-by-side and stacked now — in stacked,
  // editor-view is already full pane width, so TabBar being its own first
  // child reads identically to the old dedicated "moves down between chat
  // and editor" copy this replaces.
  const editorViewNode = (
    <div
      key="editor-view"
      ref={editorViewRef}
      data-editor-view=""
      hidden={editorViewHidden}
      className={cn(
        'relative flex min-h-0 flex-col overflow-hidden bg-pane-background',
        Boolean(pane.chatId) && presentation !== 'tabs' ? 'shrink grow-0' : 'w-full flex-1',
      )}
      style={{
        ...(Boolean(pane.chatId) && presentation !== 'tabs'
          ? // splitSizes is always [chatPct, editorPct] — see the chat
            // view's own note above.
            { flexBasis: `${splitSizes[1]}%` }
          : undefined),
        // The IDE sector reads as its OWN card next to the chat's — a
        // flat-edged (never rounded) border on whichever edge actually
        // touches the chat (left or right in side-by-side, depending on
        // which side the chat itself is on; top in stacked). Every other
        // edge draws no border of its own — see buildInnerViewStyle's own
        // doc for why.
        ...editorInnerViewStyle,
      }}
    >
      {chatVisibleAlongsideEditor && (
        <TabBar paneId={pane.id} wsId={chatWsId} onTabClick={handleTabClick} />
      )}
      <div className="relative min-h-0 flex-1 overflow-hidden">{editorViewInner}</div>
    </div>
  )

  return (
    <div
      ref={containerRef}
      data-pane-container
      data-pane-id={pane.id}
      // Which VIEW this pane belongs to (types/pane.ts). Panes carrying the
      // same value were deliberately merged into one view; panes carrying
      // different ones are separate views that happen to be tiled beside each
      // other. Published because that distinction is otherwise invisible from
      // the outside — the layout tree looks identical either way — which is
      // exactly what made "clicking a row appends to my current view" so hard
      // to see, and to check.
      data-view-id={pane.viewId ?? undefined}
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
        //
        // Dropped the moment the SplitDropOverlay below is drawing this
        // pane's own zone: that rectangle already says "this one, on that
        // side", and the same rule the file-drag ring above follows applies —
        // two concentric secondary borders is a double border, not a clearer
        // signal.
        !internalHoverZone && 'data-[pane-hit]:ring-2 data-[pane-hit]:ring-secondary',
      )}
      // Pane activation on mousedown is wired up as a real native listener
      // in the effect above, not a React prop here — see its own doc for
      // why (the portal boundary a React `onMouseDownCapture` prop can't
      // cross). `onClick` stays a React prop: it only needs to catch a
      // keyboard-triggered click (Enter/Space on a focused control inside
      // this pane), which carries no preceding mousedown at all.
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
        // ONE shared box, ROUNDED, bordered and shadowed together (via
        // `overflow-hidden` + `paneContentStyle`'s radius/border/margin) —
        // not just the content alone. Before this, TabBar sat outside this
        // div, against the unstyled `data-pane-container` shell, and showed
        // the page body's translucent --chrome-bg tint through it: a
        // two-tone "header band over rounded content" look, not the design's
        // single `.pane` surface.
        //
        // Paints the chat's own translucent `bg-chrome-bg` fill — chats/pane
        // redesign feedback: the fill needs to live HERE, on the box that
        // actually encloses both the chat and the IDE sector, not
        // separately on each region that happens not to be opaque (the chat
        // view, the chat/editor sash) — that patchwork left visible seams
        // at every boundary a caller forgot to cover explicitly (reported
        // live: a border-coloured gap at the sash, and again at this box's
        // own rounded corner). The IDE sector still reads fully opaque: its
        // own box (TabBar's row, the editor view) paints `bg-pane-background`
        // OVER this fill within its own bounds — see those files' own docs.
        // Only the chat's own area, and any uncovered sliver of this shared
        // box (the sash, a rounded corner), shows this translucency, real
        // --chrome-bg vibrancy reaching the transparent window behind it.
        // Rounding/clipping still lives here regardless of what paints a
        // fill — `overflow-hidden` clips to the radius either way.
        data-pane-content=""
        // This box's own border is now permanently neutral --border; the
        // active-pane accent rides the `data-pane-accent` overlay rendered
        // after it (see `paneAccentStyle`). `transition-colors` stays for the
        // background — the accent's fade moved to the overlay's opacity,
        // because fading a COLOUR on this particular surface (large, rounded,
        // translucent over window vibrancy) repaints all of it every frame.
        className="relative z-[1] flex min-h-0 flex-1 flex-col overflow-hidden bg-pane-chrome-bg transition-colors duration-150"
        style={paneContentStyle}
      >
        {/* Spans the WHOLE pane only when a single surface fills 100% of it
            (no chat at all, chatFillsPane, or the collapsed 'tabs'
            presentation) — never in side by side/stacked, where the redesign
            draws chat and IDE sector as two SEPARATE boxes side by side, and
            a header spanning both was the actual bug (tabs visibly sitting
            over the chat's own column too). See ChatColumnHeader/TabBar's
            OWN copy further down for the side-by-side/stacked case, each
            confined to its own column. `chatFillsPane` replaces TabBar
            entirely with ChatOnlyPaneHeader here — safe to swap (unlike the
            chat-view/editor-view divs below), since neither component holds
            any live state a remount would lose. */}
        {/* Wrapped in the chat's own resolved workspace context whether or
            not chatFillsPane — ChatOnlyPaneHeader and TabBar's own
            ChatTabItem (showChatTab) both need the chat's OWN workspace, not
            whichever one is ambient (same note as the chat-view provider
            below). For a chatless pane `chatStore` falls back to
            `ambientStore` (see its own derivation above), so this is a
            no-op wrap in that case — identical to not wrapping at all. */}
        {showTopLevelHeader && (
          <WorkspaceStoreContext.Provider value={chatStore}>
            {chatFillsPane ? (
              <ChatOnlyPaneHeader pane={pane} wsId={chatWsId} />
            ) : (
              <TabBar
                paneId={pane.id}
                wsId={chatWsId}
                onTabClick={handleTabClick}
                showChatTab={showChatTab}
              />
            )}
          </WorkspaceStoreContext.Provider>
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
            a hypothetical either — a chat drop onto a chatless pane
            (`dropChatOnPane`) fills a pane that may already be showing
            editor tabs, so this is reachable, and directly against spec
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
          {/* chatIsFirst decides visual order (sidebar on the right puts the
              chat second, next to it) — each node keeps its own stable key
              regardless of which slot it renders in, so this is never a
              remount. */}
          {chatIsFirst ? (
            <>
              {chatViewNode}
              {sashNode}
              {editorViewNode}
            </>
          ) : (
            <>
              {editorViewNode}
              {sashNode}
              {chatViewNode}
            </>
          )}
        </div>
      </div>
      {/* The active-pane accent ring. Sits OVER `data-pane-content`'s own
          neutral border (`z-[2]` to its `z-[1]`, and after it in source), on
          exactly that box's border box, so at full opacity it simply hides the
          neutral line — `--secondary` is opaque, so the two never blend.
          Childless and transparent inside, so the 150ms opacity fade it
          replaced `transition-colors`' border-colour fade with costs the
          compositor a ring, not a repaint of the whole translucent pane (see
          `paneAccentStyle`). `pointer-events-none` keeps it out of every
          gesture the pane below it owns, and it is decoration, so it is hidden
          from assistive technology. */}
      <div
        data-pane-accent=""
        aria-hidden="true"
        className="pointer-events-none absolute z-[2] transition-opacity duration-150"
        style={{ ...paneAccentStyle, opacity: showActiveBorder ? 1 : 0 }}
      />
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
