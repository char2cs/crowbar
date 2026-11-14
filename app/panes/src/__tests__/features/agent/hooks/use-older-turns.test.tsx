import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useOlderTurns } from '@/features/agent/hooks/use-older-turns'
import { loadOlder } from '@/features/agent/stores/chat-feed'
import { chatStore } from '@/features/agent/stores/chat-store'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'

vi.mock('@/features/agent/stores/chat-feed', () => ({ loadOlder: vi.fn() }))

const scroller = (scrollTop: number) => {
  const el = document.createElement('div')
  Object.defineProperty(el, 'clientHeight', { value: 400 })
  el.scrollTop = scrollTop
  return { current: el }
}
const holding = (from: number) =>
  chatStore.setState({ feeds: { 'chat-1': { ...emptyFeed(), status: 'live', from } } })

beforeEach(() => {
  vi.mocked(loadOlder).mockClear()
  holding(40)
})

describe('useOlderTurns', () => {
  it('asks for older turns at open and when the view comes to rest near the first loaded row, never mid-gesture', () => {
    renderHook(() => useOlderTurns('chat-1', scroller(120), () => {}))
    expect(loadOlder).toHaveBeenCalledTimes(1)
    const ref = scroller(5000)
    renderHook(() => useOlderTurns('chat-1', ref, () => {}))
    ref.current.scrollTop = 700 // a fling passing through: its write would cancel the momentum
    ref.current.dispatchEvent(new Event('scroll'))
    expect(loadOlder).toHaveBeenCalledTimes(1)
    ref.current.dispatchEvent(new Event('scrollend'))
    expect(loadOlder).toHaveBeenCalledTimes(2)
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
