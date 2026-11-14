import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useRecentsChat } from '@/components/sidebar/lib/use-recents-chat'
import { seedChats, writeChat } from '@/__tests__/__fixtures__/agent-chat'

describe('useRecentsChat', () => {
  it('reads the title, workspace and working flag of the chat', () => {
    seedChats([{ id: 'c1', title: 'Hello', workspaceId: 'w2', working: true }])
    const { result } = renderHook(() => useRecentsChat('w1', 'c1'))
    expect(result.current).toEqual({ id: 'c1', title: 'Hello', workspaceId: 'w2', working: true })
  })

  it('is null for a chat the store does not hold', () => {
    seedChats([])
    const { result } = renderHook(() => useRecentsChat('w1', 'gone'))
    expect(result.current).toBeNull()
  })

  it('renders only when what the row draws changes', () => {
    seedChats([{ id: 'c1', title: 'Hello', workspaceId: 'w1' }])
    let renders = 0
    const { result } = renderHook(() => {
      renders += 1
      return useRecentsChat('w1', 'c1')
    })
    const before = renders
    act(() => writeChat({ id: 'c1', liveRunnerId: 'r9' }))
    expect(renders).toBe(before)
    act(() => writeChat({ id: 'c1', title: 'Renamed' }))
    expect(result.current?.title).toBe('Renamed')
  })
})
