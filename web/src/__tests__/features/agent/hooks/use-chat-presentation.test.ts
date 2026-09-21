/**
 * `use-chat-presentation.ts` drives two splits: the terminal split hosted
 * inside `agent-chat-pane.tsx` (chat ⇄ terminal — its end-to-end behaviour is
 * covered by `agent-chat-pane-split.test.tsx`; the persistence of WHICH one a
 * chat lands back on is covered directly below, since it lives entirely in
 * this hook) and, generalized here, the pane-level chat-view ⇄ editor-view
 * split spec §7.2 describes, consumed by `pane-container.tsx`.
 *
 * `usePaneViewPresentation` is pure geometry over `editorOpen` (Task 1's
 * `PaneGroup.editorOpen`) — there is nothing to "choose", so unlike
 * `useChatPresentation` it has no `setPresentation`/`chosen` pair to test.
 */
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  SPLIT_SIDE_BY_SIDE_MIN_PX,
  useChatPresentation,
  usePaneViewPresentation,
} from '@/features/agent/hooks/use-chat-presentation'
import { useSettingsStore } from '@/features/settings/store'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'

function ref(size: { clientWidth?: number; clientHeight?: number }) {
  return { current: size as unknown as HTMLElement }
}

describe('useChatPresentation — a chat remembers its own surface', () => {
  beforeEach(() => {
    useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  })

  afterEach(() => {
    useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  })

  it('keeps a chat on Terminal across switching to another chat and back, in one mount', () => {
    const container = { current: null }
    const { result, rerender } = renderHook(
      ({ chatId }) => useChatPresentation(chatId, container),
      { initialProps: { chatId: 'chat-a' } },
    )
    expect(result.current.presentation).toBe('chat') // the global default

    act(() => result.current.setPresentation('terminal'))
    expect(result.current.presentation).toBe('terminal')

    // Switch to a different chat in the same pane — the ordinary re-seed path
    // (a runner move, or picking another tab) that already ran before this fix.
    rerender({ chatId: 'chat-b' })
    expect(result.current.presentation).toBe('chat')

    // ...and back to chat A. Before this fix `chosen` was thrown away on every
    // re-seed unless it was 'split', so this landed back on the global default
    // ('chat') instead of the 'terminal' the user had actually left it on.
    rerender({ chatId: 'chat-a' })
    expect(result.current.presentation).toBe('terminal')
  })

  it('keeps a chat on Terminal across a real unmount/remount — a Recents row reopening a closed chat', () => {
    const containerA = { current: null }
    const { result: first, unmount: unmountFirst } = renderHook(() =>
      useChatPresentation('chat-a', containerA),
    )
    act(() => first.current.setPresentation('terminal'))
    expect(first.current.presentation).toBe('terminal')

    // Closing a chat's view deletes its pane outright (`closePane`,
    // pane-slice.ts) rather than hiding it — this pane's own React state,
    // including `chosen`, dies with it.
    unmountFirst()

    // Another chat's pane mounts and unmounts in between, same as a user
    // browsing elsewhere before coming back.
    const containerB = { current: null }
    const { unmount: unmountSecond } = renderHook(() => useChatPresentation('chat-b', containerB))
    unmountSecond()

    // Reopening chat A from its Recents row mounts a BRAND NEW AgentChatPane —
    // a fresh `renderHook`, not a rerender of the same instance. The chat's own
    // choice must survive this, exactly like the bug report says: "the chat
    // mode should never be lost."
    const containerA2 = { current: null }
    const { result: second } = renderHook(() => useChatPresentation('chat-a', containerA2))
    expect(second.current.presentation).toBe('terminal')
  })
})

describe('usePaneViewPresentation', () => {
  it('is tabs when the split is off, regardless of size', () => {
    const { result } = renderHook(() =>
      usePaneViewPresentation(false, ref({ clientWidth: 2000, clientHeight: 2000 })),
    )
    expect(result.current).toBe('tabs')
  })

  it('below the side-by-side floor on both axes, tabs is the only presentation even with the split on', () => {
    const { result } = renderHook(() => usePaneViewPresentation(true, ref({ clientWidth: 600 })))
    expect(result.current).toBe('tabs')
  })

  it('wide enough and landscape presents side by side', () => {
    const { result } = renderHook(() =>
      usePaneViewPresentation(true, ref({ clientWidth: SPLIT_SIDE_BY_SIDE_MIN_PX + 100 })),
    )
    expect(result.current).toBe('side-by-side')
  })

  it('tall enough and portrait presents stacked', () => {
    const { result } = renderHook(() =>
      usePaneViewPresentation(
        true,
        ref({ clientWidth: 500, clientHeight: SPLIT_SIDE_BY_SIDE_MIN_PX + 200 }),
      ),
    )
    expect(result.current).toBe('stacked')
  })

  it('an unmeasured (0x0) container defaults to side-by-side rather than flashing to tabs first', () => {
    const { result } = renderHook(() => usePaneViewPresentation(true, { current: null }))
    expect(result.current).toBe('side-by-side')
  })

  it('a square pane at exactly the floor is side by side (width >= height wins ties)', () => {
    const { result } = renderHook(() =>
      usePaneViewPresentation(
        true,
        ref({
          clientWidth: SPLIT_SIDE_BY_SIDE_MIN_PX,
          clientHeight: SPLIT_SIDE_BY_SIDE_MIN_PX,
        }),
      ),
    )
    expect(result.current).toBe('side-by-side')
  })
})

/**
 * The measurement must not turn a pixel-by-pixel resize back into a render per
 * pixel. `usePaneViewPresentation` answers with one of three arrangements, but
 * it used to hold the raw `clientWidth`/`clientHeight` in state — so a pane
 * sash drag, which rewrites flex-basis on every raw pointermove, re-rendered
 * the whole pane subtree (PaneContainer, the transcript's Plate editor, the tab
 * bar, every Tooltip under it) once per pixel for an answer that never changed.
 * Live-measured in the dev app, a ~2s drag committed 157 renders; only the
 * handful that actually cross a threshold are legitimate.
 */
describe('usePaneViewPresentation — a resize that does not change the answer does not re-render', () => {
  function withObservedResize(run: (fire: () => void) => void) {
    const callbacks: ResizeObserverCallback[] = []
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(cb: ResizeObserverCallback) {
          callbacks.push(cb)
        }
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    )
    try {
      run(() => {
        act(() => {
          for (const cb of callbacks) cb([], {} as ResizeObserver)
        })
      })
    } finally {
      vi.unstubAllGlobals()
    }
  }

  it('stays at one render across a whole drag inside the same bucket, and still switches at the threshold', () => {
    withObservedResize((fire) => {
      const box = { clientWidth: 1200, clientHeight: 900 }
      const container = ref(box)
      let renders = 0
      const { result } = renderHook(() => {
        renders++
        return usePaneViewPresentation(true, container)
      })

      expect(result.current).toBe('side-by-side')
      const rendersAtRest = renders

      // A drag's worth of width changes, every one of them still landscape and
      // still above the side-by-side floor.
      for (let w = 1200; w > 900; w -= 5) {
        box.clientWidth = w
        fire()
      }
      expect(result.current).toBe('side-by-side')
      expect(renders).toBe(rendersAtRest)

      // Crossing a real threshold still lands, immediately.
      box.clientWidth = 700
      box.clientHeight = 500
      fire()
      expect(result.current).toBe('tabs')
      expect(renders).toBeGreaterThan(rendersAtRest)
    })
  })
})
