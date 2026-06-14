import '../monaco/monaco-environment'
import '../monaco/language-contributions'
import 'monaco-editor/min/vs/editor/editor.main.css'
import '../styles/monaco-editor.css'

import type React from 'react'
import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useEditorScroll } from '@/features/editor/hooks/use-scroll'
import {
  useWorkspaceStore,
} from '@/features/workspace/stores/workspace-context'
import { useSettingsStore } from '@/features/settings/store'
import { useEditorSettingsStore } from '@/features/editor/stores/settings-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { useEditorAppStore } from '@/features/editor/stores/editor-app-store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { hasTextContent } from '@/features/panes/types/pane-content'
import type {
  EditorCoordinateResolver,
  EditorModelPositionResolver,
} from '@/features/editor/view-model/view-layout'
import { editorAPI } from '../extensions/api'
import { ScrollDebugOverlay } from './debug/scroll-debug-overlay'
import { EditorStylesheet } from './stylesheet'
import Breadcrumb, { type BreadcrumbProps } from './toolbar/breadcrumb'
import FindBar from './toolbar/find-bar'
import { PaneLspLayer } from './pane-lsp-layer'
import { PaneEditorStateBridge } from './pane-editor-state-bridge'
import type { PaneOverlayMouseHandlers } from './pane-overlay-handlers'
import { usePaneEditorController } from '../hooks/use-pane-editor-controller'
import { usePaneEditorSatellites } from '../hooks/use-pane-editor-satellites'
import { defineMonacoTheme } from '../monaco/define-theme'
import { toEditorPosition, toEditorRange } from '../monaco/editor-conversions'
import { createRafCoalescer } from '../lib/raf-coalesce'
import type * as Monaco from 'monaco-editor'

export interface EditorSurfaceProps {
  paneId: string
  bufferId: string
  isActiveSurface?: boolean
  isPreview?: boolean
  onPromote?: () => void
  showToolbar?: boolean
  className?: string
  breadcrumbProps?: BreadcrumbProps
}

/**
 * Stable per-pane editor shell. Mounted ONCE per pane (keyed by `paneId` by the
 * parent). A buffer/tab switch is driven imperatively by
 * {@link usePaneEditorController} (model swap + content/cursor seam) and the
 * retained widget's satellite concerns are retargeted by
 * {@link usePaneEditorSatellites} — NOT by remounting this subtree.
 *
 * Crucially this component's RENDER reads NO active-buffer state
 * (`activeBufferId`/`value`/`filePath`). Everything buffer-dependent lives in
 * leaf children that each subscribe independently, so a tab switch updates only
 * those leaves instead of reconciling this whole subtree:
 *  - {@link Breadcrumb} self-resolves the active path via `paneId`.
 *  - {@link PaneLspLayer} owns LSP + in-file search + go-to-line, subscribing to
 *    the active-editor registry (`filePath`); it reads buffer CONTENT imperatively
 *    (no render subscription) so a keystroke never re-renders it.
 *  - {@link PaneEditorStateBridge} mirrors the active buffer's identity into the
 *    shared editor-state store (status-bar view-key + legacy seam).
 */
export function EditorSurface({
  paneId,
  bufferId,
  isActiveSurface = true,
  isPreview = false,
  onPromote,
  showToolbar = true,
  className,
  breadcrumbProps,
}: EditorSurfaceProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const overlayContainerRef = useRef<HTMLDivElement>(null)
  const codeLensRef = useRef<HTMLDivElement>(null)
  const renameInputRef = useRef<HTMLDivElement>(null)
  const editorCoordinateResolverRef = useRef<EditorCoordinateResolver | null>(null)
  const editorModelPositionResolverRef = useRef<EditorModelPositionResolver | null>(null)
  const mouseHandlersRef = useRef<PaneOverlayMouseHandlers | null>(null)

  const workspaceStore = useWorkspaceStore()
  const editorManager = workspaceStore.editorManager
  const registry = workspaceStore.activeEditorRegistry

  const { setRefs, setCursorAndSelection } = useEditorStateStore.use.actions()

  const zoomLevel = useZoomStore.use.editorZoomLevel()

  const enableInteractiveServices = isActiveSurface

  // ── Imperative buffer-switch controller (model swap + content seam) ───────
  // Cursor/selection sync is rAF-COALESCED off the per-cursor hot path: a burst
  // of cursor moves / keystrokes schedules a single trailing frame that reads
  // the editor's CURRENT position+selection once and writes them in ONE batched
  // store update. The pending frame is cancelled on dispose.
  const flushCursorSync = useCallback(() => {
    const editor = editorManager.getRawEditor(paneId) as
      | Monaco.editor.IStandaloneCodeEditor
      | null
    const model = editor?.getModel()
    if (!editor || !model) return
    const position = editor.getPosition()
    if (!position) return
    const selection = editor.getSelection()
    setCursorAndSelection(
      toEditorPosition(model, position),
      selection ? toEditorRange(model, selection) : undefined,
      { ensureVisible: false },
    )
  }, [editorManager, paneId, setCursorAndSelection])

  const flushCursorSyncRef = useRef(flushCursorSync)
  flushCursorSyncRef.current = flushCursorSync
  // One coalescer per pane mount; its closure reads the latest flush via ref so
  // it survives buffer swaps. Cancel the pending frame on unmount.
  const cursorSyncerRef = useRef<ReturnType<typeof createRafCoalescer> | null>(null)
  if (!cursorSyncerRef.current) {
    cursorSyncerRef.current = createRafCoalescer(() => flushCursorSyncRef.current())
  }
  useEffect(() => {
    const syncer = cursorSyncerRef.current
    return () => syncer?.cancel()
  }, [])

  const syncCursorAndSelection = useCallback(() => {
    cursorSyncerRef.current?.schedule()
  }, [])

  // mountPane wires the retained widget into the slot, sets the viewport ref,
  // applies the initial theme and installs the rAF-debounced ResizeObserver +
  // pane-resize-end layout (suppressed during a drag) — once per pane.
  const resizeCleanupRef = useRef<(() => void) | null>(null)
  const mountPane = useCallback(
    (container: HTMLElement) => {
      editorManager.mountPane(paneId, container)
      editorAPI.setTextareaRef(null)
      editorAPI.setViewportRef(container as HTMLDivElement)

      const raw = editorManager.getRawEditor(paneId) as
        | Monaco.editor.IStandaloneCodeEditor
        | null
      // Initial theme to avoid a flash; the satellites theme effect (which also
      // subscribes to theme changes) is authoritative right after mount.
      const editorSettingsTheme = useEditorSettingsStore.getState().theme
      raw?.updateOptions({
        theme: defineMonacoTheme(useSettingsStore.getState().settings.theme || editorSettingsTheme),
      })

      let layoutRafId: number | null = null
      let needsLayoutAfterResize = false
      const runLayout = () => editorManager.layoutPane(paneId)
      const resizeObserver = new ResizeObserver(() => {
        if (document.documentElement.hasAttribute('data-pane-resizing')) {
          needsLayoutAfterResize = true
          return
        }
        if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
        layoutRafId = requestAnimationFrame(() => {
          layoutRafId = null
          runLayout()
        })
      })
      resizeObserver.observe(container)

      const handlePaneResizeEnd = () => {
        if (!needsLayoutAfterResize) return
        needsLayoutAfterResize = false
        if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
        layoutRafId = requestAnimationFrame(() => {
          layoutRafId = null
          runLayout()
        })
      }
      window.addEventListener('pane-resize-end', handlePaneResizeEnd)

      resizeCleanupRef.current = () => {
        resizeObserver.disconnect()
        if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
        window.removeEventListener('pane-resize-end', handlePaneResizeEnd)
      }
    },
    // Theme is read fresh via getState(); mount-once per pane.
    [editorManager, paneId],
  )

  const unmountPane = useCallback(() => {
    resizeCleanupRef.current?.()
    resizeCleanupRef.current = null
    editorAPI.setViewportRef(null)
    editorManager.unmountPane(paneId)
  }, [editorManager, paneId])

  // Persist a content change, folding preview-promote-on-first-edit in. Skipped
  // when this surface isn't active (matches legacy CodeEditor) or when the edit
  // is the echo of a genuine external change applied by the satellites.
  const isPreviewRef = useRef(isPreview)
  isPreviewRef.current = isPreview
  const onPromoteRef = useRef(onPromote)
  onPromoteRef.current = onPromote
  const isActiveSurfaceRef = useRef(isActiveSurface)
  isActiveSurfaceRef.current = isActiveSurface
  const externalApplyRef = useRef<string | null>(null)
  const { handleContentChange } = useEditorAppStore.use.actions()
  const onContentChange = useCallback(
    (content: string) => {
      if (externalApplyRef.current === content) {
        externalApplyRef.current = null
        return
      }
      if (!isActiveSurfaceRef.current) return
      if (isPreviewRef.current) onPromoteRef.current?.()
      void handleContentChange(content, undefined, undefined, undefined, {
        contentAlreadyApplied: false,
      })
    },
    [handleContentChange],
  )

  const selectActiveBuffer = useCallback(
    (state: import('@/features/workspace/stores/workspace-store.types').WorkspaceState) => {
      const id = state.panes[paneId]?.activeBufferId ?? null
      const buffer = id ? state.buffers.find((b) => b.id === id) : null
      if (!buffer || !hasTextContent(buffer)) return null
      return { filePath: buffer.path }
    },
    [paneId],
  )

  usePaneEditorController(paneId, containerRef, {
    store: workspaceStore,
    selectActiveBuffer,
    manager: editorManager,
    registry,
    mountPane,
    unmountPane,
    onContentChange,
    syncCursorAndSelection,
  })

  // ── Retained-widget satellite concerns (settings, theme, decorations, LSP) ─
  const syncLspOverlayTransform = useCallback((scrollTop: number, scrollLeft: number) => {
    const transform = `translate(-${scrollLeft}px, -${scrollTop}px)`
    for (const ref of [codeLensRef, renameInputRef]) {
      if (ref.current) ref.current.style.transform = transform
    }
  }, [])

  const handleCoordinateResolverChange = useCallback(
    (resolver: EditorCoordinateResolver | null) => {
      editorCoordinateResolverRef.current = resolver
    },
    [],
  )
  const handleModelPositionResolverChange = useCallback(
    (resolver: EditorModelPositionResolver | null) => {
      editorModelPositionResolverRef.current = resolver
    },
    [],
  )

  usePaneEditorSatellites(paneId, {
    onScrollOffsetChange: syncLspOverlayTransform,
    onCoordinateResolverChange: handleCoordinateResolverChange,
    onModelPositionResolverChange: handleModelPositionResolverChange,
    isActiveSurface,
    readOnly: false,
    scrollable: true,
    externalApplyRef,
  })

  // ── Editor-state store refs (active surface only) ──────────────────────────
  useEffect(() => {
    if (!isActiveSurface) return
    setRefs({ editorRef: overlayContainerRef })
  }, [isActiveSurface, setRefs])

  // Stable resolvers forwarded to the LSP layer; read the live model via refs.
  const resolveEditorPosition = useCallback<EditorCoordinateResolver>(
    (clientX, clientY) => editorCoordinateResolverRef.current?.(clientX, clientY) ?? null,
    [],
  )
  const resolveModelPosition = useCallback<EditorModelPositionResolver>(
    (line, column) => editorModelPositionResolverRef.current?.(line, column) ?? null,
    [],
  )

  // Stable container mouse handlers — forward to the LSP layer's latest set so a
  // buffer switch never changes the container's handler identity.
  const handleMouseMove = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    mouseHandlersRef.current?.handleMouseMove(e)
  }, [])
  const handleMouseLeave = useCallback(() => {
    mouseHandlersRef.current?.handleMouseLeave()
  }, [])
  const handleMouseEnter = useCallback(() => {
    mouseHandlersRef.current?.handleMouseEnter()
  }, [])
  const handleClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    mouseHandlersRef.current?.handleClick(e)
  }, [])

  useEditorScroll(overlayContainerRef, null)

  const interactiveHandlers = useMemo(
    () =>
      enableInteractiveServices
        ? {
            onMouseMove: handleMouseMove,
            onMouseLeave: handleMouseLeave,
            onMouseEnter: handleMouseEnter,
            onClick: handleClick,
          }
        : {},
    [enableInteractiveServices, handleMouseMove, handleMouseLeave, handleMouseEnter, handleClick],
  )

  // `bufferId` is the parent's stable-mount hint (the surface keys off paneId and
  // its leaves read the active buffer reactively); referenced to satisfy
  // noUnusedParameters.
  void bufferId

  return (
    <>
      <EditorStylesheet />
      <PaneEditorStateBridge
        paneId={paneId}
        isActiveSurface={isActiveSurface}
        onContentChange={onContentChange}
      />
      <div className="absolute inset-0 flex flex-col overflow-hidden">
        {showToolbar && <Breadcrumb {...breadcrumbProps} paneId={paneId} />}

        {showToolbar && enableInteractiveServices && <FindBar />}

        <div
          ref={overlayContainerRef}
          className={`editor-container relative min-h-0 flex-1 overflow-hidden ${className || ''}`}
          data-zoom-level={zoomLevel}
          style={{ scrollbarWidth: 'none', msOverflowStyle: 'none' }}
          {...interactiveHandlers}
        >
          <PaneLspLayer
            paneId={paneId}
            isActiveSurface={isActiveSurface}
            overlayContainerRef={overlayContainerRef}
            mouseHandlersRef={mouseHandlersRef}
            resolveEditorPosition={resolveEditorPosition}
            resolveModelPosition={resolveModelPosition}
            syncLspOverlayTransform={syncLspOverlayTransform}
            codeLensRef={codeLensRef}
            renameInputRef={renameInputRef}
          />

          {/* Stable Monaco slot — the retained per-pane widget mounts here. */}
          <div className="absolute inset-0 bg-background">
            <div
              ref={containerRef}
              className="absolute inset-0"
              data-monaco-editor-scroll
            />
          </div>
        </div>
      </div>

      {enableInteractiveServices && <ScrollDebugOverlay />}
    </>
  )
}

export default EditorSurface
