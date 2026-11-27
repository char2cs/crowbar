import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  presetChatLandingPresentation,
  useChatPresentation,
  usePaneViewPresentation,
} from '@/features/agent/hooks/use-chat-presentation'
import { chatStore } from '@/features/agent/stores/chat-store'
import { useSettingsStore } from '@/features/settings/store'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'

const platform = vi.hoisted(() => ({ mobile: false }))
const SIDE_BY_SIDE_MIN_PX = 780

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

describe('useChatPresentation — on the phone', () => {
  beforeEach(() => {
    useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  })

  afterEach(() => {
    platform.mobile = false
    chatStore.setState({ presentations: {} })
  })

  it('lands on the chat even where the desktop default is the terminal', () => {
    platform.mobile = true
    const { result } = renderHook(() => useChatPresentation('chat-m', { current: null }))
    expect(result.current.presentation).toBe('chat')
  })

  it('honors a terminal pick and remembers it for that chat', () => {
    platform.mobile = true
    const container = { current: null }
    const { result, rerender } = renderHook(
      ({ chatId }) => useChatPresentation(chatId, container),
      { initialProps: { chatId: 'chat-m' } },
    )
    act(() => result.current.setPresentation('terminal'))
    expect(result.current.presentation).toBe('terminal')

    rerender({ chatId: 'chat-other' })
    expect(result.current.presentation).toBe('chat')
    rerender({ chatId: 'chat-m' })
    expect(result.current.presentation).toBe('terminal')
  })

  it('never offers or shows the split', () => {
    platform.mobile = true
    useSettingsStore.setState({
      settings: { ...getDefaultSettingsSnapshot(), chatSplitPresentationEnabled: true },
    })
    const { result } = renderHook(() => useChatPresentation('chat-s', { current: null }))
    act(() => result.current.setPresentation('split'))
    expect(result.current.splitEnabled).toBe(false)
    expect(result.current.presentation).toBe('chat')
  })
})

describe('useChatPresentation — a view the daemon holds', () => {
  const withSplit = (enabled: boolean) =>
    useSettingsStore.setState({
      settings: { ...getDefaultSettingsSnapshot(), chatSplitPresentationEnabled: enabled },
    })
  afterEach(() => {
    platform.mobile = false
    chatStore.setState({ presentations: {} })
    withSplit(false)
  })
  const render = (held: 'chat' | 'terminal') =>
    renderHook(() => useChatPresentation('chat-h', { current: null }, held)).result

  it('shows the terminal it holds over any pick, split included', () => {
    withSplit(true)
    presetChatLandingPresentation('chat-h', 'chat')
    const result = render('terminal')
    expect(result.current.presentation).toBe('terminal')
    act(() => result.current.setPresentation('split'))
    expect(result.current.presentation).toBe('terminal')
  })

  it('shows the chat it holds over a terminal pick, and a split only while split exists', () => {
    withSplit(true)
    presetChatLandingPresentation('chat-h', 'terminal')
    const result = render('chat')
    expect(result.current.presentation).toBe('chat')
    act(() => result.current.setPresentation('split'))
    expect(result.current.presentation).toBe('split')
    act(() => withSplit(false))
    expect(result.current.presentation).toBe('chat')
  })

  it('leaves the user on the chat it held when a switch moves the chat to a provider that holds none', () => {
    const { result, rerender } = renderHook(
      ({ held }: { held: 'chat' | null }) => useChatPresentation('chat-h', { current: null }, held),
      { initialProps: { held: 'chat' as 'chat' | null } },
    )
    expect(result.current.presentation).toBe('chat')
    rerender({ held: null })
    expect(result.current.presentation).toBe('chat')
  })

  it('shows the terminal it holds on the phone too', () => {
    platform.mobile = true
    expect(render('terminal').current.presentation).toBe('terminal')
  })
})

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

  it('keeps a chat on its own pick across switching to another chat and back, in one mount', () => {
    const container = { current: null }
    const { result, rerender } = renderHook(
      ({ chatId }) => useChatPresentation(chatId, container),
      { initialProps: { chatId: 'chat-a' } },
    )
    expect(result.current.presentation).toBe('chat')

    act(() => result.current.setPresentation('terminal'))
    expect(result.current.presentation).toBe('terminal')

    rerender({ chatId: 'chat-b' })
    expect(result.current.presentation).toBe('chat')

    rerender({ chatId: 'chat-a' })
    expect(result.current.presentation).toBe('terminal')
  })

  it('keeps a chat on its own pick across a real unmount/remount — a Recents row reopening a closed chat', () => {
    const containerA = { current: null }
    const { result: first, unmount: unmountFirst } = renderHook(() =>
      useChatPresentation('chat-a', containerA),
    )
    act(() => first.current.setPresentation('chat'))
    expect(first.current.presentation).toBe('chat')

    unmountFirst()

    const containerB = { current: null }
    const { unmount: unmountSecond } = renderHook(() => useChatPresentation('chat-b', containerB))
    unmountSecond()

    const containerA2 = { current: null }
    const { result: second } = renderHook(() => useChatPresentation('chat-a', containerA2))
    expect(second.current.presentation).toBe('chat')
  })
})

describe('presetChatLandingPresentation — choosing a new chat’s surface at creation', () => {
  beforeEach(() => {
    chatStore.setState({ presentations: {} })
    useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  })

  afterEach(() => {
    chatStore.setState({ presentations: {} })
    useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  })

  it('lands a specific brand-new chat on Terminal, with the global default untouched', () => {
    expect(getDefaultSettingsSnapshot().chatIsDefaultPresentation).toBe(true)

    presetChatLandingPresentation('new-chat-1', 'terminal')

    const container = { current: null }
    const { result } = renderHook(() => useChatPresentation('new-chat-1', container))
    expect(result.current.presentation).toBe('terminal')

    expect(useSettingsStore.getState().settings.chatIsDefaultPresentation).toBe(true)
    const otherContainer = { current: null }
    const { result: other } = renderHook(() =>
      useChatPresentation('some-other-chat', otherContainer),
    )
    expect(other.current.presentation).toBe('chat')
  })

  it('is a one-time seed: an explicit pick made afterward still wins', () => {
    presetChatLandingPresentation('new-chat-2', 'chat')
    const container = { current: null }
    const { result } = renderHook(() => useChatPresentation('new-chat-2', container))
    expect(result.current.presentation).toBe('chat')

    act(() => result.current.setPresentation('terminal'))
    expect(result.current.presentation).toBe('terminal')
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
      usePaneViewPresentation(true, ref({ clientWidth: SIDE_BY_SIDE_MIN_PX + 100 })),
    )
    expect(result.current).toBe('side-by-side')
  })

  it('tall enough and portrait presents stacked', () => {
    const { result } = renderHook(() =>
      usePaneViewPresentation(
        true,
        ref({ clientWidth: 500, clientHeight: SIDE_BY_SIDE_MIN_PX + 200 }),
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
          clientWidth: SIDE_BY_SIDE_MIN_PX,
          clientHeight: SIDE_BY_SIDE_MIN_PX,
        }),
      ),
    )
    expect(result.current).toBe('side-by-side')
  })
})

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

      for (let w = 1200; w > 900; w -= 5) {
        box.clientWidth = w
        fire()
      }
      expect(result.current).toBe('side-by-side')
      expect(renders).toBe(rendersAtRest)

      box.clientWidth = 700
      box.clientHeight = 500
      fire()
      expect(result.current).toBe('tabs')
      expect(renders).toBeGreaterThan(rendersAtRest)
    })
  })
})
