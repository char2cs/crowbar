import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'

vi.mock('@/features/agent/lib/kill-agent', () => ({ confirmAndKillAgent: vi.fn() }))

import { useKillAgentMenuItems } from '@/features/agent/hooks/use-kill-agent-menu-items'
import { confirmAndKillAgent } from '@/features/agent/lib/kill-agent'
import { runChat } from '@/__tests__/__fixtures__/running-chats'
import { resetEntities } from '@/__tests__/__fixtures__/sidebar-entities'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const runThread = (viewers = 0) => runChat('c1', { title: 'A thread', viewers })

beforeEach(() => {
  vi.clearAllMocks()
  resetHostWorld()
  resetEntities()
})

describe('useKillAgentMenuItems', () => {
  it('offers nothing for a chat whose agent is not running', () => {
    const { result } = renderHook(() => useKillAgentMenuItems('c1'))

    expect(result.current).toEqual([])
  })

  it('offers nothing without a chat', () => {
    runThread()

    const { result } = renderHook(() => useKillAgentMenuItems(undefined))

    expect(result.current).toEqual([])
  })

  it('kills the chat’s agent, with its viewer count, once chosen', () => {
    runThread(2)
    const { result } = renderHook(() => useKillAgentMenuItems('c1'))

    result.current.find((item) => item.id === 'kill-agent')!.onClick()

    expect(confirmAndKillAgent).toHaveBeenCalledWith(
      expect.objectContaining({ hostId: 'local', chatId: 'c1', workspaceId: 'ws-1', viewers: 2 }),
    )
  })

  it("kills a paired host's agent on that host", async () => {
    pairRemoteHost('host-1')
    await act(async () => {})
    act(() => runChat('c-r', { hostId: 'host-1' }))
    const { result } = renderHook(() => useKillAgentMenuItems('c-r'))

    result.current.find((item) => item.id === 'kill-agent')!.onClick()

    expect(confirmAndKillAgent).toHaveBeenCalledWith(
      expect.objectContaining({ hostId: 'host-1', chatId: 'c-r' }),
    )
  })
})
