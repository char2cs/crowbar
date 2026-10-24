import { describe, expect, it, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { createStore } from 'zustand'
import { createRef } from 'react'
import {
  usePaneEditorController,
  type PaneEditorControllerDeps,
} from '@/features/editor/hooks/use-pane-editor-controller'
import type { ActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'
import type {
  ActiveBufferInfo,
  PaneSwitchManager,
} from '@/features/editor/lib/pane-editor-controller'
import { documentUri } from '@/features/editor/documents/document-ref'
import { workspaceRef } from '@/lib/api/documents'

interface TestState {
  activeBufferId: string | null
  buffers: Record<string, ActiveBufferInfo>
  setActive(id: string | null): void
}

const A = workspaceRef('w1', '/a.ts')
const B = workspaceRef('w1', '/b.ts')

function makeStore() {
  return createStore<TestState>((set) => ({
    activeBufferId: 'a',
    buffers: { a: { doc: A }, b: { doc: B } },
    setActive: (id) => set({ activeBufferId: id }),
  }))
}

/** Fake editor whose listeners are captured so a test can fire them. */
function makeEditor() {
  const handlers: Record<string, () => void> = {}
  const listen = (key: string) => (cb: () => void) => {
    handlers[key] = cb
    return { dispose: vi.fn() }
  }
  return {
    getModel: () => ({}),
    onDidChangeModelContent: listen('content'),
    onDidChangeCursorSelection: listen('cursor'),
    fire: (key: string) => handlers[key]?.(),
  }
}

// The registry is a window singleton; a fresh module instance per test isolates it.
let freshRegistry: ActiveEditorRegistry
beforeEach(async () => {
  vi.resetModules()
  freshRegistry = (await import('@/features/editor/lib/active-editor-context')).activeEditors
})

/** Every document is open except those in `closed`; `open(uri)` opens one. */
function makeDeps(store: ReturnType<typeof makeStore>, editor: ReturnType<typeof makeEditor>) {
  const registry = freshRegistry
  const closed = new Set<string>()
  const openListeners = new Set<(uri: string) => void>()
  const manager: PaneSwitchManager = {
    showBuffer: vi.fn((_paneId: string, uri: string) => !closed.has(uri)),
    getRawEditor: vi.fn(() => editor),
  }
  const deps: PaneEditorControllerDeps<TestState> = {
    store,
    selectActiveBuffer: (s) => (s.activeBufferId ? s.buffers[s.activeBufferId] : null),
    manager,
    registry,
    onDidOpen: (listener) => {
      openListeners.add(listener)
      return () => openListeners.delete(listener)
    },
    mountPane: vi.fn(),
    unmountPane: vi.fn(),
    syncCursorAndSelection: vi.fn(),
  }
  const open = (uri: string) => {
    closed.delete(uri)
    for (const listener of openListeners) listener(uri)
  }
  return { deps, registry, manager, closed, open, openListeners }
}

describe('usePaneEditorController', () => {
  function setup(closed: string[] = []) {
    const store = makeStore()
    const editor = makeEditor()
    const made = makeDeps(store, editor)
    for (const uri of closed) made.closed.add(uri)
    const containerRef = createRef<HTMLElement>()
    ;(containerRef as { current: HTMLElement }).current = document.createElement('div')
    const view = renderHook(() => usePaneEditorController('p1', containerRef, made.deps))
    return { store, editor, view, ...made }
  }

  it('mounts the pane once and applies the initial buffer', () => {
    const { deps, manager } = setup()
    expect(deps.mountPane).toHaveBeenCalledTimes(1)
    expect(manager.showBuffer).toHaveBeenCalledWith('p1', documentUri(A))
    expect(deps.registry.get('p1')?.filePath).toBe('/a.ts')
  })

  it('swaps the model imperatively on activeBufferId change (no remount)', () => {
    const { store, deps, manager } = setup()
    ;(manager.showBuffer as ReturnType<typeof vi.fn>).mockClear()

    act(() => store.getState().setActive('b'))

    expect(manager.showBuffer).toHaveBeenCalledWith('p1', documentUri(B))
    expect(deps.mountPane).toHaveBeenCalledTimes(1) // still mounted once
    expect(deps.registry.get('p1')?.filePath).toBe('/b.ts')
  })

  it('does not swap when an unrelated store change keeps activeBufferId the same', () => {
    const { store, manager } = setup()
    ;(manager.showBuffer as ReturnType<typeof vi.fn>).mockClear()
    act(() => store.setState({ buffers: { ...store.getState().buffers } })) // churn, same active
    expect(manager.showBuffer).not.toHaveBeenCalled()
  })

  it('edits and caret moves sync the cursor', () => {
    const { editor, deps } = setup()
    act(() => editor.fire('content'))
    act(() => editor.fire('cursor'))
    expect(deps.syncCursorAndSelection).toHaveBeenCalledTimes(2)
  })

  it('shows a document that opens after its tab became active', () => {
    const { deps, manager, open } = setup([documentUri(A)])
    expect(deps.registry.get('p1')).toBeUndefined()

    act(() => open(documentUri(A)))

    expect(manager.showBuffer).toHaveBeenLastCalledWith('p1', documentUri(A))
    expect(deps.registry.get('p1')?.filePath).toBe('/a.ts')
  })

  it('does NOT re-mount when a re-render hands it fresh callbacks', () => {
    const store = makeStore()
    const { deps, manager } = makeDeps(store, makeEditor())
    const containerRef = createRef<HTMLElement>()
    ;(containerRef as { current: HTMLElement }).current = document.createElement('div')
    const view = renderHook((d) => usePaneEditorController('p1', containerRef, d), {
      initialProps: deps,
    })
    expect(deps.mountPane).toHaveBeenCalledTimes(1)

    view.rerender({ ...deps, syncCursorAndSelection: vi.fn() })

    expect(deps.unmountPane).not.toHaveBeenCalled()
    expect(deps.mountPane).toHaveBeenCalledTimes(1)
    expect(manager.showBuffer).toHaveBeenCalledTimes(1) // no re-apply either
  })

  it('cleanup unmounts the pane, clears the registry and stops listening', () => {
    const { deps, view, openListeners } = setup()
    expect(deps.registry.get('p1')).toBeDefined()
    view.unmount()
    expect(deps.unmountPane).toHaveBeenCalledTimes(1)
    expect(deps.registry.get('p1')).toBeUndefined()
    expect(openListeners.size).toBe(0)
  })

  // R12 regression: closing the LAST tab must clear the pane's registry entry,
  // or satellites keep a disposed model's context ('Model is disposed!').
  it('clears the registry when the last tab closes (active buffer -> null)', () => {
    const { store, deps } = setup()
    expect(deps.registry.get('p1')?.filePath).toBe('/a.ts')

    act(() => store.getState().setActive(null))

    expect(deps.registry.get('p1')).toBeUndefined()
  })

  // R12 regression: reopening the SAME file after a close re-publishes the context.
  it('re-publishes the context when the same file is reopened after a close', () => {
    const { store, deps } = setup()

    act(() => store.getState().setActive(null))
    expect(deps.registry.get('p1')).toBeUndefined()

    act(() => store.getState().setActive('a'))
    expect(deps.registry.get('p1')?.filePath).toBe('/a.ts')
  })
})
