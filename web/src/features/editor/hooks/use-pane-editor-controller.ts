/**
 * usePaneEditorController — imperative per-pane editor switch controller.
 *
 * Decouples a buffer/tab switch from React rendering. The pane's retained Monaco
 * widget is mounted ONCE (manager.mountPane) and its content/cursor/scroll
 * listeners + the ContentSink are bound ONCE per pane. A tab switch is then a
 * purely imperative model swap driven by a NARROW `store.subscribe` to
 * `panes[paneId].activeBufferId` — it does NOT go through `useStore`/React render.
 *
 * On each swap the controller:
 *  1. calls {@link applyActiveBuffer} → `manager.showBuffer(paneId, fileUri(path))`
 *     and publishes the new ActiveEditorContext to the active-editor registry, and
 *  2. rebinds the once-registered listeners' target to the new model via a ref
 *     (the `onDidChangeModelContent` etc. handlers are attached once to the
 *     retained editor and read the editor's CURRENT model).
 *
 * Behaviors preserved from the old monaco-editor.tsx controller effect:
 *  - Model-authoritative content via a throttled ContentSink (delayMs 150).
 *  - First-edit-after-swap synchronous flush (immediate dirty + preview promote).
 *  - Flush on blur, on the `flush-editor-content` window event, on switch-away,
 *    and on unmount (flush then dispose).
 *  - Cursor/selection sync to the editor state store (status bar) on edit and
 *    on cursor-selection change.
 *  - Scroll-offset forwarding (no manual view-state cache write for managed panes).
 *  - Preview-promote on first edit (folded into onContentChange by the caller).
 */

import { useEffect, useRef } from 'react'
import type { StoreApi } from 'zustand'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { ContentSink } from '@/features/editor/lib/content-sink'
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

/**
 * The monaco editor surface the controller drives. Kept structural (not a hard
 * `monaco` import) so the hook can be unit-tested and stays swap-tolerant — the
 * listeners are attached once and always read `getModel()` for the live model.
 */
export interface ControlledEditor {
  getModel(): { getValue(): string } | null
  onDidChangeModelContent(cb: () => void): Disposable
  onDidBlurEditorText(cb: () => void): Disposable
  onDidChangeCursorSelection(cb: () => void): Disposable
}

export interface PaneEditorControllerDeps<S> {
  /** Vanilla workspace store (for narrow activeBufferId subscription). */
  store: SubscribableStore<S>
  /** Reads the active buffer for this pane from a store snapshot, or null. */
  selectActiveBuffer(state: S): ActiveBufferInfo | null
  manager: PaneSwitchManager
  registry: ActiveEditorRegistry
  /** Mounts the pane's retained widget into the container (idempotent). */
  mountPane(container: HTMLElement): void
  /** Unmounts/disposes the pane's retained widget. */
  unmountPane(): void
  /**
   * Persist a content change (model text) to the buffer store. This is the
   * single write seam; the caller folds preview-promote-on-first-edit into it.
   */
  onContentChange(value: string): void
  /** Sync cursor/selection to the editor state store (status bar). */
  syncCursorAndSelection(): void
  /** Trailing-debounce window for the ContentSink. */
  sinkDelayMs?: number
}

/**
 * Mount + drive the retained widget for `paneId`. Effect deps are `[paneId]`
 * only (everything else is read through the latest-deps ref) so the widget is
 * mounted and the listeners are bound exactly once per pane lifetime.
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
    const { store, mountPane, unmountPane, manager, registry } = depsRef.current

    mountPane(container)

    const editor = manager.getRawEditor(paneId) as ControlledEditor | null

    // The ContentSink coalesces keystrokes (trailing debounce) and forwards the
    // latest model text to the buffer store fire-and-forget. The FIRST edit
    // after each swap is flushed synchronously (immediate dirty + preview
    // promote); sustained typing thereafter is throttled.
    let firstEditFlushed = false
    const sink = new ContentSink({
      delayMs: depsRef.current.sinkDelayMs ?? 150,
      write: (value) => depsRef.current.onContentChange(value),
    })

    const flush = () => sink.flush()
    window.addEventListener('flush-editor-content', flush)

    const disposables: Disposable[] = []
    if (editor) {
      disposables.push(
        editor.onDidChangeModelContent(() => {
          const model = editor.getModel()
          if (!model) return
          sink.push(model.getValue())
          if (!firstEditFlushed) {
            firstEditFlushed = true
            sink.flush()
          }
          depsRef.current.syncCursorAndSelection()
        }),
        editor.onDidBlurEditorText(() => sink.flush()),
        editor.onDidChangeCursorSelection(() => depsRef.current.syncCursorAndSelection()),
      )
    }

    // Imperative switch: swap the model + publish context, then reset the
    // first-edit-flush latch so the next buffer's first edit flushes immediately.
    let currentUri: string | null = null
    const applySwitch = () => {
      const buffer = depsRef.current.selectActiveBuffer(store.getState())
      const nextUri = buffer ? fileUri(buffer.filePath) : null
      if (nextUri === currentUri) return
      // Flush the outgoing buffer's pending edit before swapping away.
      sink.flush()
      applyActiveBuffer({ manager, registry }, paneId, buffer)
      currentUri = nextUri
      firstEditFlushed = false
    }

    // Initial buffer, then narrow subscription to activeBufferId changes only.
    applySwitch()
    const unsubscribe = store.subscribe(applySwitch)

    return () => {
      unsubscribe()
      window.removeEventListener('flush-editor-content', flush)
      // Persist any pending throttled edit, then drop the timer.
      sink.flush()
      sink.dispose()
      for (const d of disposables) d.dispose()
      unmountPane()
      registry.clear(paneId)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [paneId])
}
