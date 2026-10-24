/**
 * usePaneEditorController — the pane's retained Monaco widget mounts once, and a tab
 * switch is an imperative model swap deduped on the document uri, outside React render.
 * A document that opens after its tab became active is shown when it opens.
 */

import { useEffect, useRef } from 'react'
import type { StoreApi } from 'zustand'
import { documentUri } from '@/features/editor/documents/document-ref'
import {
  applyActiveBuffer,
  type ActiveBufferInfo,
  type PaneSwitchManager,
} from '@/features/editor/lib/pane-editor-controller'
import type { ActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'

/** Anything with vanilla zustand `getState`/`subscribe`. */
type SubscribableStore<S> = Pick<StoreApi<S>, 'getState' | 'subscribe'>

/** Disposable returned by monaco listener registrations. */
interface Disposable {
  dispose(): void
}

/** The monaco editor events the controller listens to (structural, so fakes fit). */
interface ControlledEditor {
  onDidChangeModelContent(cb: () => void): Disposable
  onDidChangeCursorSelection(cb: () => void): Disposable
}

export interface PaneEditorControllerDeps<S> {
  /** Vanilla pane store, read for the pane's active buffer. */
  store: SubscribableStore<S>
  /** Reads the active buffer for this pane from a store snapshot, or null. */
  selectActiveBuffer(state: S): ActiveBufferInfo | null
  manager: PaneSwitchManager
  registry: ActiveEditorRegistry
  /** Subscribes to documents opening; answers the unsubscribe. */
  onDidOpen(listener: (uri: string) => void): () => void
  /** Mounts the pane's retained widget into the container (idempotent). */
  mountPane(container: HTMLElement): void
  /** Unmounts/disposes the pane's retained widget. */
  unmountPane(): void
  /** Sync cursor/selection to the editor state store (status bar). */
  syncCursorAndSelection(): void
}

/**
 * Mount + drive the retained widget for `paneId`. The effect depends on the
 * pane alone (everything else is read through the latest-deps ref), so the
 * widget is mounted and the listeners are bound once per pane lifetime.
 */
export function usePaneEditorController<S>(
  paneId: string,
  containerRef: React.RefObject<HTMLElement | null>,
  deps: PaneEditorControllerDeps<S>,
): void {
  // Latest deps in a ref so the mount effect never re-runs on identity changes
  // of callbacks/selectors; the once-registered listeners read through it.
  const depsRef = useRef(deps)
  depsRef.current = deps

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const { store, mountPane, unmountPane, manager, registry, onDidOpen } = depsRef.current

    mountPane(container)

    const editor = manager.getRawEditor(paneId) as ControlledEditor | null
    const sync = () => depsRef.current.syncCursorAndSelection()
    const disposables: Disposable[] = editor
      ? [editor.onDidChangeModelContent(sync), editor.onDidChangeCursorSelection(sync)]
      : []

    // The uri the widget shows; null while the active document is not open yet.
    let currentUri: string | null = null
    const applySwitch = () => {
      const buffer = depsRef.current.selectActiveBuffer(store.getState())
      const nextUri = buffer ? documentUri(buffer.doc) : null
      if (nextUri === currentUri) return
      const shown = buffer ? applyActiveBuffer({ manager, registry }, paneId, buffer) : undefined
      // The outgoing context names a model the pane no longer shows (or that is gone).
      if (!shown && currentUri) registry.clearIfActive(paneId, currentUri)
      currentUri = shown ? nextUri : null
    }

    applySwitch()
    const unsubscribe = store.subscribe(applySwitch)
    const stopOpen = onDidOpen(applySwitch)

    return () => {
      unsubscribe()
      stopOpen()
      for (const d of disposables) d.dispose()
      unmountPane()
      registry.clear(paneId)
    }
  }, [paneId, containerRef])
}
