import '../monaco/monaco-environment'
import '../monaco/language-contributions'
import 'monaco-editor/min/vs/editor/editor.main.css'
import '../styles/monaco-editor.css'

import { useCallback, useEffect, useRef } from 'react'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useSettingsStore } from '@/features/settings/store'
import { useEditorSettingsStore } from '@/features/editor/stores/settings-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { setBufferContent } from '@/features/editor/lib/buffer-save'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { hasTextContent } from '@/features/panes/types/pane-content'
import { registerLspProviders } from '../lsp/monaco-lsp-providers'
import { EditorStylesheet } from './stylesheet'
import Breadcrumb, { type BreadcrumbProps } from './toolbar/breadcrumb'
import { PaneEditorStateBridge } from './pane-editor-state-bridge'
import { usePaneEditorController } from '../hooks/use-pane-editor-controller'
import { usePaneEditorSatellites } from '../hooks/use-pane-editor-satellites'
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

/**
 * How often the cursor/selection store write is allowed to land WHILE a
 * selection drag is in flight.
 *
 * Monaco emits one selection change per pointer move and per auto-scroll tick —
 * ~120/s on this display — and each one used to schedule a store write that
 * React turned into a commit through the app's whole provider chain. The only
 * things that RENDER from it are the status bar's `line:col` chip and the
 * completion popup (which is never open mid-drag); everything else reads the
 * store imperatively, after the gesture. Measured live in the Tauri app on an
 * 896-line file: 200 selection changes cost 76fps with a commit each and 83fps
 * with none, and a real drag-select went 89 → 94 fps median (50 → 11 commits).
 * 100ms keeps the chip visibly live at 10Hz — past what the eye resolves on a
 * moving caret — and `flush()` on pointer-up lands the exact final position, so
 * nothing downstream ever sees a stale value at rest.
 */
const SELECTION_DRAG_SYNC_MS = 100

export interface EditorSurfaceProps {
  paneId: string
  bufferId: string
  /**
   * The workspace THIS buffer belongs to (buffer.workspaceId), NOT the ambient
   * WorkspaceStoreContext. WorkspaceHost keeps every retained WorkspaceView
   * mounted at once for keep-alive, each rendering the same window-level pane
   * tree under a DIFFERENT ambient context — resolving the EditorManager from
   * ambient context instead of the buffer's own would let a wrong-ambient
   * hidden copy mount a second, leaked Monaco model/widget under a manager
   * the buffer's own `closeBuffer` cleanup (scoped to buf.workspaceId, see
   * buffer-slice.ts's `editorManagerFor`) never visits. Passed explicitly by
   * EditorPane, which already looked the buffer up to arm this exact
   * workspace's editor before mounting this surface.
   */
  workspaceId: string
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
 *  - {@link PaneEditorStateBridge} mirrors the active buffer's identity into the
 *    shared editor-state store (status-bar view-key + legacy seam).
 */
// react-doctor-disable-next-line no-giant-component -- accepted: cohesive editor surface — hosts one Monaco viewport plus its overlays/resize observer sharing the same editor ref; splitting fragments that ref coordination.
export function EditorSurface({
  paneId,
  bufferId,
  workspaceId,
  isActiveSurface = true,
  isPreview = false,
  onPromote,
  showToolbar = true,
  className,
  breadcrumbProps,
}: EditorSurfaceProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const overlayContainerRef = useRef<HTMLDivElement>(null)

  // Resolved by the buffer's OWN workspace id (see the `workspaceId` prop
  // doc), not ambient context. Non-null: EditorPane awaits
  // `getWorkspaceStore(workspaceId)?.armEditor()` for this same workspaceId
  // before it mounts EditorSurface (that is the lazy-Monaco seam), so the
  // store and its manager are always present here.
  const workspaceStore = getWorkspaceStore(workspaceId)!
  const editorManager = workspaceStore.editorManager!
  const registry = workspaceStore.activeEditorRegistry

  const { setRefs, setCursorAndSelection } = useEditorStateStore.use.actions()

  const zoomLevel = useZoomStore.use.editorZoomLevel()

  // ── Imperative buffer-switch controller (model swap + content seam) ───────
  // Cursor/selection sync is rAF-COALESCED off the per-cursor hot path: a burst
  // of cursor moves / keystrokes schedules a single trailing frame that reads
  // the editor's CURRENT position+selection once and writes them in ONE batched
  // store update. The pending frame is cancelled on dispose.
  const flushCursorSync = useCallback(() => {
    const editor = editorManager.getRawEditor(paneId) as Monaco.editor.IStandaloneCodeEditor | null
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
      editorManager.mountPane(paneId, container)

      const raw = editorManager.getRawEditor(paneId) as Monaco.editor.IStandaloneCodeEditor | null
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

      // A selection drag is in flight: GPU-promote Monaco so per-frame
      // selection-overlay updates are compositor-composited rather than triggering
      // WKWebView CPU tile re-rasterization across each selected line (the CSS
      // half, keyed on `data-editor-selecting`), and throttle the cursor/selection
      // store write the drag would otherwise fire at pointer-move rate (the JS
      // half, via `isSelectionDragging` in the coalescer above).
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
    [editorManager, paneId],
  )

  const unmountPane = useCallback(() => {
    resizeCleanupRef.current?.()
    resizeCleanupRef.current = null
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

  // Shared write core. `targetBufferId` (when known) pins the write to a SPECIFIC
  // buffer so a flush during a fast tab switch attributes content to the edited
  // buffer, not the now-active one (I3). When omitted, the write targets this
  // pane's active buffer.
  const writeContent = useCallback(
    (content: string, targetBufferId?: string) => {
      if (externalApplyRef.current === content) {
        externalApplyRef.current = null
        return
      }
      if (!isActiveSurfaceRef.current) return
      if (isPreviewRef.current) onPromoteRef.current?.()
      const bufferId =
        targetBufferId ?? windowPaneStore.getState().panes[paneId]?.activeEditorTabId ?? null
      if (bufferId) setBufferContent(bufferId, content)
    },
    [paneId],
  )

  // Legacy 5-arg seam handed to the state bridge / editorAPI (no bufferId →
  // active buffer). Identity is stable; extra legacy args are ignored here.
  const onContentChange = useCallback((content: string) => writeContent(content), [writeContent])

  // Controller seam: the imperative ContentSink flush passes the buffer it was
  // tracking so the write targets the edited buffer (I3 fix).
  const onControllerContentChange = useCallback(
    (content: string, bufferId: string | null) => writeContent(content, bufferId ?? undefined),
    [writeContent],
  )

  const selectActiveBuffer = useCallback(
    (state: import('@/features/panes/stores/window-pane-store.types').WindowPaneState) => {
      const id = state.panes[paneId]?.activeEditorTabId ?? null
      const buffer = id ? state.buffers.find((b) => b.id === id) : null
      // Text-content buffers always carry a real path (see OpenEditorTabSpec) —
      // skip publishing an active-buffer switch rather than key Monaco's model
      // registry by an undefined uri if that invariant is ever violated.
      if (!buffer || !hasTextContent(buffer) || !buffer.path) return null
      // `workspaceId` here is THIS SURFACE'S OWN resolved workspace (the prop
      // above), not `buffer.workspaceId` — see ActiveBufferInfo's own doc:
      // the model uri must agree with whichever workspace's armEditor()
      // closure will be asked for this uri's content, which is always the
      // manager this surface is CURRENTLY mounted on.
      return { bufferId: buffer.id, filePath: buffer.path, workspaceId }
    },
    [paneId, workspaceId],
  )

  usePaneEditorController(
    paneId,
    containerRef,
    {
      store: windowPaneStore,
      selectActiveBuffer,
      manager: editorManager,
      registry,
      mountPane,
      unmountPane,
      onContentChange: onControllerContentChange,
      syncCursorAndSelection,
    },
    // The manager instance itself, NOT the workspace id — see the hook's own
    // doc for why a string proxy missed a real regression: `destroyWorkspaceStore`
    // can dispose and recreate this same workspace's EditorManager (a fresh
    // instance) without `workspaceId` ever changing, and only the manager
    // reference actually distinguishes "still the one this pane is mounted on"
    // from "was replaced out from under it."
    editorManager,
  )

  // ── Retained-widget satellite concerns (settings, theme, LSP document sync) ─
  usePaneEditorSatellites(paneId, {
    registry,
    editorManager,
    workspaceId,
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

  // The toolbar's search button opens Monaco's own find widget.
  const openFind = useCallback(() => {
    const editor = editorManager.getRawEditor(paneId) as Monaco.editor.IStandaloneCodeEditor | null
    void editor?.getAction('actions.find')?.run()
  }, [editorManager, paneId])

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
        registry={registry}
      />
      <div className="absolute inset-0 flex flex-col overflow-hidden">
        {/* `bufferId` passed explicitly — see EditorHostRegistry's own doc:
            this EditorSurface can now be the pane's RETAINED editor while a
            non-editor tab (branch review, ...) is the pane's actual active
            tab. Breadcrumb's own `paneId`-only fallback resolves via
            `pane.activeEditorTabId`, which would then name the OTHER tab —
            live-caught as the breadcrumb reading "branch-review://..." while
            still showing this file's content. `bufferId` is always the
            buffer THIS surface is actually showing, active tab or not. */}
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

export default EditorSurface
