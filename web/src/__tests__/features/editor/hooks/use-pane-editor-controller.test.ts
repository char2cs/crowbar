import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { createStore } from 'zustand'
import { createRef } from 'react'
import {
  usePaneEditorController,
  type ControlledEditor,
  type PaneEditorControllerDeps,
} from '@/features/editor/hooks/use-pane-editor-controller'
import {
  createActiveEditorRegistry,
  type ActiveEditorRegistry,
} from '@/features/editor/lib/active-editor-context'
import type { PaneSwitchManager } from '@/features/editor/lib/pane-editor-controller'
import { fileUri } from '@/features/editor/lib/editor-uri'

interface TestState {
  activeBufferId: string | null
  buffers: Record<string, { bufferId: string; filePath: string }>
  setActive(id: string | null): void
}

function makeStore() {
  return createStore<TestState>((set) => ({
    activeBufferId: 'a',
    buffers: {
      a: { bufferId: 'a', filePath: '/a.ts' },
      b: { bufferId: 'b', filePath: '/b.ts' },
    },
    setActive: (id) => set({ activeBufferId: id }),
  }))
}

/** Fake controlled editor whose listeners are captured so we can fire them. */
function makeEditor() {
  const handlers: Record<string, () => void> = {}
  let modelValue = ''
  const editor: ControlledEditor & { fire(k: string): void; setModelValue(v: string): void } = {
    getModel: () => ({ getValue: () => modelValue }),
    onDidChangeModelContent: (cb) => {
      handlers.content = cb
      return { dispose: vi.fn() }
    },
    onDidBlurEditorText: (cb) => {
      handlers.blur = cb
      return { dispose: vi.fn() }
    },
    onDidChangeCursorSelection: (cb) => {
      handlers.cursor = cb
      return { dispose: vi.fn() }
    },
    fire: (k) => handlers[k]?.(),
    setModelValue: (v) => {
      modelValue = v
    },
  }
  return editor
}

function makeDeps(store: ReturnType<typeof makeStore>, editor: ReturnType<typeof makeEditor>) {
  const registry: ActiveEditorRegistry = createActiveEditorRegistry()
  const manager: PaneSwitchManager = {
    showBuffer: vi.fn(),
    getRawEditor: vi.fn(() => editor),
  }
  const deps: PaneEditorControllerDeps<TestState> = {
    store,
    selectActiveBuffer: (s) => (s.activeBufferId ? s.buffers[s.activeBufferId] : null),
    manager,
    registry,
    mountPane: vi.fn(),
    unmountPane: vi.fn(),
    onContentChange: vi.fn(),
    syncCursorAndSelection: vi.fn(),
    sinkDelayMs: 50,
  }
  return { deps, registry, manager }
}

describe('usePaneEditorController', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  function setup() {
    const store = makeStore()
    const editor = makeEditor()
    const { deps, manager } = makeDeps(store, editor)
    const containerRef = createRef<HTMLElement>()
    ;(containerRef as { current: HTMLElement }).current = document.createElement('div')
    const view = renderHook(() => usePaneEditorController('p1', containerRef, deps))
    return { store, editor, deps, manager, view }
  }

  it('mounts the pane once and applies the initial buffer', () => {
    const { deps, manager } = setup()
    expect(deps.mountPane).toHaveBeenCalledTimes(1)
    expect(manager.showBuffer).toHaveBeenCalledWith('p1', fileUri('/a.ts'))
    expect(deps.registry.get('p1')?.filePath).toBe('/a.ts')
  })

  it('swaps the model imperatively on activeBufferId change (no remount)', () => {
    const { store, deps, manager } = setup()
    ;(manager.showBuffer as ReturnType<typeof vi.fn>).mockClear()

    act(() => store.getState().setActive('b'))

    expect(manager.showBuffer).toHaveBeenCalledWith('p1', fileUri('/b.ts'))
    expect(deps.mountPane).toHaveBeenCalledTimes(1) // still mounted once
    expect(deps.registry.get('p1')?.filePath).toBe('/b.ts')
  })

  it('does not swap when an unrelated store change keeps activeBufferId the same', () => {
    const { store, manager } = setup()
    ;(manager.showBuffer as ReturnType<typeof vi.fn>).mockClear()
    act(() => store.setState({ buffers: { ...store.getState().buffers } })) // churn, same active
    expect(manager.showBuffer).not.toHaveBeenCalled()
  })

  it('throttles edits through the sink and flushes the FIRST edit synchronously', () => {
    const { editor, deps } = setup()
    editor.setModelValue('x')
    act(() => editor.fire('content'))
    // First edit flushed immediately, attributed to the active buffer 'a'.
    expect(deps.onContentChange).toHaveBeenCalledWith('x', 'a')
    expect(deps.syncCursorAndSelection).toHaveBeenCalled()

    ;(deps.onContentChange as ReturnType<typeof vi.fn>).mockClear()
    editor.setModelValue('xy')
    act(() => editor.fire('content'))
    expect(deps.onContentChange).not.toHaveBeenCalled() // throttled
    act(() => vi.advanceTimersByTime(50))
    expect(deps.onContentChange).toHaveBeenCalledWith('xy', 'a')
  })

  it('flushes pending edits on blur and on the flush-editor-content event', () => {
    const { editor, deps } = setup()
    act(() => editor.fire('content')) // first edit flush (empty)
    ;(deps.onContentChange as ReturnType<typeof vi.fn>).mockClear()

    editor.setModelValue('blurred')
    act(() => editor.fire('content')) // queued (throttled)
    act(() => editor.fire('blur'))
    expect(deps.onContentChange).toHaveBeenCalledWith('blurred', 'a')

    ;(deps.onContentChange as ReturnType<typeof vi.fn>).mockClear()
    editor.setModelValue('saved')
    act(() => editor.fire('content'))
    act(() => window.dispatchEvent(new Event('flush-editor-content')))
    expect(deps.onContentChange).toHaveBeenCalledWith('saved', 'a')
  })

  it('flushes the outgoing buffer before swapping away', () => {
    const { store, editor, deps } = setup()
    act(() => editor.fire('content')) // first-edit flush
    ;(deps.onContentChange as ReturnType<typeof vi.fn>).mockClear()
    editor.setModelValue('pending')
    act(() => editor.fire('content')) // queued, not yet written
    act(() => store.getState().setActive('b'))
    expect(deps.onContentChange).toHaveBeenCalledWith('pending', 'a')
  })

  // I3 regression: a fast tab switch must NOT misattribute the outgoing buffer's
  // pending text to the now-active buffer. Type into A, switch to B within the
  // throttle window; the flushed text must be written to A, never to B.
  it('attributes pending content to the EDITED buffer on a fast switch, not the new one', () => {
    const { store, editor, deps } = setup()
    act(() => editor.fire('content')) // first-edit flush for 'a'
    ;(deps.onContentChange as ReturnType<typeof vi.fn>).mockClear()

    // Type into A (queued, throttled — not yet flushed).
    editor.setModelValue('typed-into-A')
    act(() => editor.fire('content'))
    expect(deps.onContentChange).not.toHaveBeenCalled()

    // Switch to B within the throttle window → flush of A's pending text fires.
    act(() => store.getState().setActive('b'))

    const calls = (deps.onContentChange as ReturnType<typeof vi.fn>).mock.calls
    const aWrite = calls.find((c) => c[0] === 'typed-into-A')
    expect(aWrite).toBeDefined()
    expect(aWrite?.[1]).toBe('a') // attributed to A, the edited buffer
    // It must never have been written against B.
    expect(calls.some((c) => c[0] === 'typed-into-A' && c[1] === 'b')).toBe(false)
  })

  it('cleanup unmounts the pane and clears the registry', () => {
    const { deps, view } = setup()
    expect(deps.registry.get('p1')).toBeDefined()
    view.unmount()
    expect(deps.unmountPane).toHaveBeenCalledTimes(1)
    expect(deps.registry.get('p1')).toBeUndefined()
  })

  // R12 regression: closing the LAST tab (activeBufferId -> null) must CLEAR the
  // pane's registry entry. Previously applyActiveBuffer early-returned for a null
  // buffer and left the disposed model's context in place, which then crashed
  // satellites ('Model is disposed!') on the next content sync. The pane never
  // unmounts, so cleanup-on-unmount did not cover this.
  it('clears the registry when the last tab closes (active buffer -> null)', () => {
    const { store, deps } = setup()
    expect(deps.registry.get('p1')?.filePath).toBe('/a.ts')

    act(() => store.getState().setActive(null))

    expect(deps.registry.get('p1')).toBeUndefined()
  })

  // R12 regression: after a close, reopening the SAME file must re-publish the
  // context (and re-notify satellites). With the registry cleared on close this
  // is a fresh set; the model-identity dedup additionally protects the case where
  // the entry was not cleared but the model was recreated.
  it('re-publishes the context when the same file is reopened after a close', () => {
    const { store, deps } = setup()

    act(() => store.getState().setActive(null))
    expect(deps.registry.get('p1')).toBeUndefined()

    act(() => store.getState().setActive('a'))
    expect(deps.registry.get('p1')?.filePath).toBe('/a.ts')
  })
})
