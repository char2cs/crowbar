import { useEffect, useRef } from 'react'
import type { StoreApi } from 'zustand'
import { documentUri } from '@/features/editor/documents/document-ref'
import {
  applyActiveBuffer,
  type ActiveBufferInfo,
  type PaneSwitchManager,
} from '@/features/editor/lib/pane-editor-controller'
import type { ActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'

type SubscribableStore<S> = Pick<StoreApi<S>, 'getState' | 'subscribe'>

interface Disposable {
  dispose(): void
}

interface ControlledEditor {
  onDidChangeModelContent(cb: () => void): Disposable
  onDidChangeCursorSelection(cb: () => void): Disposable
}

export interface PaneEditorControllerDeps<S> {
  store: SubscribableStore<S>
  selectActiveBuffer(state: S): ActiveBufferInfo | null
  manager: PaneSwitchManager
  registry: ActiveEditorRegistry
  onDidOpen(listener: (uri: string) => void): () => void
  mountPane(container: HTMLElement): void
  unmountPane(): void
  syncCursorAndSelection(): void
}

export function usePaneEditorController<S>(
  paneId: string,
  containerRef: React.RefObject<HTMLElement | null>,
  deps: PaneEditorControllerDeps<S>,
): void {
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

    let currentUri: string | null = null
    const applySwitch = () => {
      const buffer = depsRef.current.selectActiveBuffer(store.getState())
      const nextUri = buffer ? documentUri(buffer.doc) : null
      if (nextUri === currentUri) return
      const shown = buffer ? applyActiveBuffer({ manager, registry }, paneId, buffer) : undefined
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
