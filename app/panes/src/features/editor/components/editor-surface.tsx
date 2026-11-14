import '../monaco/monaco-environment'
import '../monaco/language-contributions'
import 'monaco-editor/min/vs/editor/editor.main.css'
import '../styles/monaco-editor.css'

import { useCallback, useEffect, useRef } from 'react'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useSettingsStore } from '@/features/settings/store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf } from '@/features/editor/documents/document-ref'
import type { EditorHost } from '@/features/editor/host/editor-host'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { registerLspProviders } from '../lsp/monaco-lsp-providers'
import { EditorStylesheet } from './stylesheet'
import Breadcrumb, { type BreadcrumbProps } from './toolbar/breadcrumb'
import { usePaneEditorController } from '../hooks/use-pane-editor-controller'
import { usePaneEditorSatellites } from '../hooks/use-pane-editor-satellites'
import { useWorkspaceReadOnly } from '../hooks/use-workspace-read-only'
import { defineMonacoTheme } from '../monaco/define-theme'
import { toEditorPosition, toEditorRange } from '../monaco/editor-conversions'
import { createRafCoalescer } from '../lib/raf-coalesce'
import {
  beginSelectionDrag,
  endSelectionDrag,
  isSelectionDragging,
  releaseSelectionDrag,
} from '../lib/selection-drag'
import type * as Monaco from 'monaco-editor'

// Language features (completion, hover, rename, …) are Monaco providers over
// the daemon's /lsp routes; registered once, with the editor chunk.
registerLspProviders()

/** How often the cursor/selection store write may land during a selection drag. Monaco emits
 *  ~120 changes/s mid-drag and only the line:col chip renders from them; 10 Hz keeps it live, and
 *  `flush()` on pointer-up lands the exact final position. */
const SELECTION_DRAG_SYNC_MS = 100

export interface EditorSurfaceProps {
  paneId: string
  bufferId: string
  /** The window's editor host; EditorPane mounts this surface once it has loaded. */
  host: EditorHost
  isActiveSurface?: boolean
  showToolbar?: boolean
  className?: string
  breadcrumbProps?: BreadcrumbProps
}

/** Stable per-pane editor shell, mounted once per pane. Tab switches go through
 *  usePaneEditorController and usePaneEditorSatellites, and its render reads no active-buffer
 *  state, so only self-subscribing leaves (e.g. Breadcrumb) update on a switch. */
// react-doctor-disable-next-line no-giant-component -- accepted: cohesive editor surface — hosts one Monaco viewport plus its overlays/resize observer sharing the same editor ref; splitting fragments that ref coordination.
export function EditorSurface({
  paneId,
  bufferId,
  host,
  isActiveSurface = true,
  showToolbar = true,
  className,
  breadcrumbProps,
}: EditorSurfaceProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const overlayContainerRef = useRef<HTMLDivElement>(null)

  const { manager, documents } = host

  const { setActiveEditorViewKey, setCursorAndSelection } = useEditorStateStore.use.actions()

  const zoomLevel = useZoomStore.use.editorZoomLevel()

  // Cursor/selection sync is rAF-coalesced off the keystroke path: a burst schedules one trailing
  // frame that reads the live position and selection into one store update; cancelled on dispose.
  const flushCursorSync = useCallback(() => {
    const editor = manager.getRawEditor(paneId) as Monaco.editor.IStandaloneCodeEditor | null
    const model = editor?.getModel()
    if (!editor || !model) return
    const position = editor.getPosition()
    if (!position) return
    const selection = editor.getSelection()
    setCursorAndSelection(
      toEditorPosition(model, position),
      selection ? toEditorRange(model, selection) : undefined,
    )
  }, [manager, paneId, setCursorAndSelection])

  const flushCursorSyncRef = useRef(flushCursorSync)
  flushCursorSyncRef.current = flushCursorSync
  // One coalescer per pane mount; its closure reads the latest flush via ref so
  // it survives buffer swaps. Cancel the pending frame on unmount.
  const cursorSyncerRef = useRef<ReturnType<typeof createRafCoalescer> | null>(null)
  if (!cursorSyncerRef.current) {
    cursorSyncerRef.current = createRafCoalescer(() => flushCursorSyncRef.current(), {
      minIntervalMs: () => (isSelectionDragging() ? SELECTION_DRAG_SYNC_MS : 0),
    })
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
      manager.mountPane(paneId, container)

      const raw = manager.getRawEditor(paneId) as Monaco.editor.IStandaloneCodeEditor | null
      // Initial theme to avoid a flash; the satellites theme effect (which also
      // subscribes to theme changes) is authoritative right after mount.
      raw?.updateOptions({
        theme: defineMonacoTheme(useSettingsStore.getState().settings.theme || 'crowbar-dark'),
      })

      let layoutRafId: number | null = null
      let needsLayoutAfterResize = false
      const runLayout = () => manager.layoutPane(paneId)
      const resizeObserver = new ResizeObserver(() => {
        if (document.documentElement.hasAttribute('data-pane-resizing')) {
          needsLayoutAfterResize = true
          return
        }
        // GPU-promote Monaco for the layout frame so WKWebView rasterizes the
        // resized surface on the compositor thread instead of the main thread.
        document.documentElement.setAttribute('data-editor-layout', '1')
        if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
        layoutRafId = requestAnimationFrame(() => {
          layoutRafId = null
          runLayout()
          // Clear one frame after layout so the compositor has time to settle.
          requestAnimationFrame(() => {
            document.documentElement.removeAttribute('data-editor-layout')
          })
        })
      })
      // react-doctor-disable-next-line effect-needs-cleanup -- observer is disconnected/removed via `resizeCleanupRef` → `unmountPane`; tracer can't follow the ref-stored disposer.
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

      // During a selection drag, GPU-promote Monaco (CSS, via `data-editor-selecting`) so
      // overlay updates don't re-rasterize WKWebView tiles per line, and throttle the store write
      // the drag fires at pointer-move rate (JS, via `isSelectionDragging` in the coalescer).
      let dragHeld = false
      const handlePointerDown = (e: PointerEvent) => {
        if (e.button !== 0 || dragHeld) return
        dragHeld = true
        beginSelectionDrag()
      }
      const handlePointerUp = () => {
        if (!dragHeld) return
        dragHeld = false
        endSelectionDrag()
        // Land the final caret/selection now rather than leaving it in a
        // throttled timer — everything that reads the store on demand (jump
        // navigation, rename, the per-buffer view-state cache) reads it at rest.
        cursorSyncerRef.current?.flush()
      }
      container.addEventListener('pointerdown', handlePointerDown)
      window.addEventListener('pointerup', handlePointerUp)
      window.addEventListener('pointercancel', handlePointerUp)

      resizeCleanupRef.current = () => {
        resizeObserver.disconnect()
        if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
        window.removeEventListener('pane-resize-end', handlePaneResizeEnd)
        container.removeEventListener('pointerdown', handlePointerDown)
        window.removeEventListener('pointerup', handlePointerUp)
        window.removeEventListener('pointercancel', handlePointerUp)
        releaseSelectionDrag(dragHeld)
        dragHeld = false
        document.documentElement.removeAttribute('data-editor-layout')
      }
    },
    // Theme is read fresh via getState(); mount-once per pane.
    [manager, paneId],
  )

  const unmountPane = useCallback(() => {
    resizeCleanupRef.current?.()
    resizeCleanupRef.current = null
    manager.unmountPane(paneId)
  }, [manager, paneId])

  const selectActiveBuffer = useCallback(
    (state: import('@/features/panes/stores/window-pane-store.types').WindowPaneState) => {
      const id = state.panes[paneId]?.activeEditorTabId ?? null
      const buffer = id ? state.buffers.find((b) => b.id === id) : null
      // Only an editor buffer names a document Monaco holds a model for.
      if (!buffer || !isEditorContent(buffer) || !buffer.path) return null
      return { doc: documentRefOf(buffer) }
    },
    [paneId],
  )

  usePaneEditorController(paneId, containerRef, {
    store: windowPaneStore,
    selectActiveBuffer,
    manager,
    registry: activeEditors,
    onDidOpen: (listener) => documents.onDidOpen(listener),
    mountPane,
    unmountPane,
    syncCursorAndSelection,
  })

  const activeWorkspaceId = useStore(
    windowPaneStore,
    useCallback(
      (state: import('@/features/panes/stores/window-pane-store.types').WindowPaneState) => {
        const id = state.panes[paneId]?.activeEditorTabId
        const buffer = id ? state.buffers.find((b) => b.id === id) : undefined
        return buffer && isEditorContent(buffer) ? buffer.workspaceId : null
      },
      [paneId],
    ),
  )
  const readOnly = useWorkspaceReadOnly(activeWorkspaceId)

  // ── Retained-widget satellite concerns (settings, theme, LSP document sync) ─
  usePaneEditorSatellites(paneId, {
    registry: activeEditors,
    isActiveSurface,
    readOnly,
    scrollable: true,
  })

  // ── Status-bar cursor: which pane/buffer the mirrored cursor belongs to ───
  const activeBufferId = useStore(
    windowPaneStore,
    useCallback((state) => state.panes[paneId]?.activeEditorTabId ?? null, [paneId]),
  )
  useEffect(() => {
    if (isActiveSurface)
      setActiveEditorViewKey(activeBufferId ? `${paneId}:${activeBufferId}` : null)
  }, [activeBufferId, isActiveSurface, paneId, setActiveEditorViewKey])

  // The toolbar's search button opens Monaco's own find widget.
  const openFind = useCallback(() => {
    const editor = manager.getRawEditor(paneId) as Monaco.editor.IStandaloneCodeEditor | null
    void editor?.getAction('actions.find')?.run()
  }, [manager, paneId])

  // `bufferId` is the parent's stable-mount hint (the surface keys off paneId and
  // its leaves read the active buffer reactively); referenced to satisfy
  // noUnusedParameters.
  void bufferId

  return (
    <>
      <EditorStylesheet />

      <div className="absolute inset-0 flex flex-col overflow-hidden">
        {/* `bufferId` explicitly: a retained editor may sit behind a non-editor active tab,
            and the pane's active tab would then name the wrong buffer. */}
        {showToolbar && (
          <Breadcrumb
            {...breadcrumbProps}
            paneId={paneId}
            bufferId={bufferId}
            onFind={isActiveSurface ? openFind : undefined}
          />
        )}

        <div
          ref={overlayContainerRef}
          className={`editor-container relative min-h-0 flex-1 overflow-hidden ${className || ''}`}
          data-zoom-level={zoomLevel}
          style={{ scrollbarWidth: 'none', msOverflowStyle: 'none' }}
        >
          {/* Stable Monaco slot — the retained per-pane widget mounts here. */}
          <div className="absolute inset-0 bg-transparent">
            <div ref={containerRef} className="absolute inset-0" data-monaco-editor-scroll />
          </div>
        </div>
      </div>
    </>
  )
}
