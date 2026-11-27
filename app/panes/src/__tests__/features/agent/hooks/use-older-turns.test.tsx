import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useOlderTurns } from '@/features/agent/hooks/use-older-turns'
import { loadOlder } from '@/features/agent/stores/chat-feed'
import { chatStore } from '@/features/agent/stores/chat-store'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'

vi.mock('@/features/agent/stores/chat-feed', () => ({ loadOlder: vi.fn() }))

const scroller = (scrollTop: number, scrollHeight = 4000) => {
  const el = document.createElement('div')
  Object.defineProperty(el, 'clientHeight', { value: 400 })
  Object.defineProperty(el, 'scrollHeight', { value: scrollHeight })
  el.scrollTop = scrollTop
  return { current: el }
}
const holding = (from: number) =>
  chatStore.setState({ feeds: { 'chat-1': { ...emptyFeed(), status: 'live', from, seq: 50 } } })

beforeEach(() => {
  vi.mocked(loadOlder).mockClear()
  holding(40)
})

describe('useOlderTurns', () => {
  it('loads nothing when mounted at the bottom of a feed that can scroll', () => {
    const el = scroller(3600)
    renderHook(() => useOlderTurns('chat-1', el, () => {}))
    expect(loadOlder).not.toHaveBeenCalled()
  })

  it('loads when the view comes to rest near the first loaded row, never mid-gesture', () => {
    const ref = scroller(5000)
    renderHook(() => useOlderTurns('chat-1', ref, () => {}))
    ref.current.scrollTop = 700
    ref.current.dispatchEvent(new Event('scroll'))
    expect(loadOlder).not.toHaveBeenCalled()
    ref.current.dispatchEvent(new Event('scrollend'))
    expect(loadOlder).toHaveBeenCalledTimes(1)
    expect(loadOlder).toHaveBeenCalledWith('chat-1')
  })

  it('loads at mount while the loaded rows cannot fill the view', () => {
    const el = scroller(0, 300)
    renderHook(() => useOlderTurns('chat-1', el, () => {}))
    expect(loadOlder).toHaveBeenCalledTimes(1)
  })

  it('asks for nothing when the feed holds the chat from its start', () => {
    holding(0)
    renderHook(() => useOlderTurns('chat-1', scroller(0), () => {}))
    expect(loadOlder).not.toHaveBeenCalled()
  })

  it('keeps the reader in place when older rows land above them', () => {
    const keepPlace = vi.fn()
    const { rerender } = renderHook(() => useOlderTurns('chat-1', scroller(5000), keepPlace))
    keepPlace.mockClear()
    holding(10)
    rerender()
    expect(keepPlace).toHaveBeenCalledTimes(1)
  })
})
