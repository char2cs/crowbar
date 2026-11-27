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

registerLspProviders()

const SELECTION_DRAG_SYNC_MS = 100

export interface EditorSurfaceProps {
  paneId: string
  bufferId: string
  host: EditorHost
  isActiveSurface?: boolean
  showToolbar?: boolean
  className?: string
  breadcrumbProps?: BreadcrumbProps
}

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

  const resizeCleanupRef = useRef<(() => void) | null>(null)
  const mountPane = useCallback(
    (container: HTMLElement) => {
      manager.mountPane(paneId, container)

      const raw = manager.getRawEditor(paneId) as Monaco.editor.IStandaloneCodeEditor | null
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
        document.documentElement.setAttribute('data-editor-layout', '1')
        if (layoutRafId !== null) cancelAnimationFrame(layoutRafId)
        layoutRafId = requestAnimationFrame(() => {
          layoutRafId = null
          runLayout()
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

  usePaneEditorSatellites(paneId, {
    registry: activeEditors,
    isActiveSurface,
    readOnly,
    scrollable: true,
  })

  const activeBufferId = useStore(
    windowPaneStore,
    useCallback((state) => state.panes[paneId]?.activeEditorTabId ?? null, [paneId]),
  )
  useEffect(() => {
    if (isActiveSurface)
      setActiveEditorViewKey(activeBufferId ? `${paneId}:${activeBufferId}` : null)
  }, [activeBufferId, isActiveSurface, paneId, setActiveEditorViewKey])

  const openFind = useCallback(() => {
    const editor = manager.getRawEditor(paneId) as Monaco.editor.IStandaloneCodeEditor | null
    void editor?.getAction('actions.find')?.run()
  }, [manager, paneId])

  void bufferId

  return (
    <>
      <EditorStylesheet />

      <div className="absolute inset-0 flex flex-col overflow-hidden">
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
          <div className="absolute inset-0 bg-transparent">
            <div ref={containerRef} className="absolute inset-0" data-monaco-editor-scroll />
          </div>
        </div>
      </div>
    </>
  )
}
