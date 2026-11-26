/**
 * `use-chat-presentation.ts` drives two splits: the terminal split hosted
 * inside `agent-chat-pane.tsx` (chat ⇄ terminal — its end-to-end behaviour is
 * covered by `agent-chat-pane-split.test.tsx`; the persistence of WHICH one a
 * chat lands back on is covered directly below, since it lives entirely in
 * this hook) and, generalized here, the pane-level chat-view ⇄ editor-view
 * split spec §7.2 describes, consumed by `pane-container.tsx`.
 *
 * `usePaneViewPresentation` is pure geometry over `PaneGroup.editorOpen` — there is
 * nothing to "choose", so unlike `useChatPresentation` it has no
 * `setPresentation`/`chosen` pair to test.
 */
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  presetChatLandingPresentation,
  useChatPresentation,
} from '@/features/agent/hooks/use-chat-presentation'
import { chatStore } from '@/features/agent/stores/chat-store'
import { useSettingsStore } from '@/features/settings/store'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

// The phone lands on the chat, but the provider's TUI is one tap away and the pick sticks.
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

// A provider without hot-swap has one view; the daemon's, not a pick or a preference.
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

  // The phone opens on the chat by default, but a chat the daemon holds on its TUI is there.
  it('shows the terminal it holds on the phone too', () => {
    platform.mobile = true
    expect(render('terminal').current.presentation).toBe('terminal')
  })
})

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
    expect(result.current.presentation).toBe('chat') // the global default

    act(() => result.current.setPresentation('terminal'))
    expect(result.current.presentation).toBe('terminal')

    // Switch to a different chat in the same pane — the ordinary re-seed path
    // (a runner move, or picking another tab).
    rerender({ chatId: 'chat-b' })
    expect(result.current.presentation).toBe('chat')

    // ...and back to chat A, on the 'terminal' the user left it on, not the default.
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
    expect(second.current.presentation).toBe('chat')
  })
})

/**
 * THE BUG: "can't start chats directly on a CLI, it always obligates me to
 * use the native chat". `chatIsDefaultPresentation` (chat-presentation.ts)
 * genuinely seeds a brand-new chat's landing surface from the user's global
 * preference — that plumbing was never broken. What never existed is a way
 * to land ONE new chat on Terminal without also flipping that preference for
 * every other chat: no creation entry point (space-content-actions.ts's
 * `handleCreate`, use-pane-keyboard.ts's ⌘N) offers a choice at all, they
 * only ever accept whatever the global default happens to be.
 *
 * `presetChatLandingPresentation` is the fix's one new capability: a creation
 * call site records a chat's landing surface into the SAME map an in-pane
 * pick already writes, before that chat's pane ever mounts.
 */
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

    // The preset named exactly one chat — everyone else still lands on
    // whatever the (untouched) global default says.
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
