import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useOlderTurns } from '@/features/agent/hooks/use-older-turns'
import { loadOlder } from '@/features/agent/stores/chat-feed'
import { chatStore } from '@/features/agent/stores/chat-store'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'

vi.mock('@/features/agent/stores/chat-feed', () => ({ loadOlder: vi.fn() }))

const scroller = (scrollTop: number) => ({
  current: { scrollTop, clientHeight: 400 } as HTMLElement,
})
const holding = (from: number) =>
  chatStore.setState({ feeds: { 'chat-1': { ...emptyFeed(), status: 'live', from } } })

beforeEach(() => vi.mocked(loadOlder).mockClear())

describe('useOlderTurns', () => {
  it('asks for older turns once the reader is at the first loaded row, while some are older', () => {
    holding(40)
    const { rerender } = renderHook(
      ({ first, top }) => useOlderTurns('chat-1', first, scroller(top), () => {}),
      { initialProps: { first: 12 as number | undefined, top: 2000 } },
    )
    // Row 0 drawn by the first commit, before the view lands at the bottom.
    rerender({ first: 0, top: 2000 })
    expect(loadOlder).not.toHaveBeenCalled()
    rerender({ first: 0, top: 120 })
    expect(loadOlder).toHaveBeenCalledWith('chat-1')

    vi.mocked(loadOlder).mockClear()
    holding(0)
    rerender({ first: 0, top: 0 })
    expect(loadOlder).not.toHaveBeenCalled()
  })

  it('keeps the reader in place when older rows land above them', () => {
    holding(40)
    const keepPlace = vi.fn()
    const { rerender } = renderHook(() => useOlderTurns('chat-1', 5, scroller(900), keepPlace))
    keepPlace.mockClear()
    holding(10)
    rerender()
    expect(keepPlace).toHaveBeenCalledTimes(1)
  })
})
