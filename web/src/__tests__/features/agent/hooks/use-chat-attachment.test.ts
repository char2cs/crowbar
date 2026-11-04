import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'

const { subscribe, unsubscribe } = vi.hoisted(() => {
  const unsubscribe = vi.fn()
  return { unsubscribe, subscribe: vi.fn(() => unsubscribe) }
})
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))

import { useChatAttachment } from '@/features/agent/hooks/use-chat-attachment'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  vi.clearAllMocks()
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1' })
})

afterEach(() => __resetWorkspaceScopesForTest())

describe('useChatAttachment', () => {
  it('holds the chat’s attach socket open while mounted, and lets go on unmount', () => {
    const { unmount } = renderHook(() => useChatAttachment('ws-1', 'chat-1'))

    expect(subscribe).toHaveBeenCalledExactlyOnceWith('/v0/chats/chat-1/attach', expect.anything())
    expect(unsubscribe).not.toHaveBeenCalled()

    unmount()

    expect(unsubscribe).toHaveBeenCalledOnce()
  })

  it('moves to the new chat when the pane follows its runner', () => {
    const { rerender } = renderHook(({ id }) => useChatAttachment('ws-1', id), {
      initialProps: { id: 'chat-1' },
    })

    rerender({ id: 'chat-2' })

    expect(unsubscribe).toHaveBeenCalledOnce()
    expect(subscribe).toHaveBeenLastCalledWith('/v0/chats/chat-2/attach', expect.anything())
  })

  it('attaches to nothing without a chat', () => {
    renderHook(() => useChatAttachment('ws-1', ''))

    expect(subscribe).not.toHaveBeenCalled()
  })
})
