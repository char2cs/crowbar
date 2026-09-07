import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { useChatWorkspaceId } from '@/features/panes/hooks/use-chat-workspace-id'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { AgentChat } from '@/features/agent/api/agent-api'

const chat = (id: string, wsId: string): AgentChat => ({
  id,
  workspaceId: wsId,
  title: id,
  liveRunnerId: '',
  terminalSessionId: '',
  activeProviderId: 'claude',
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
  parentId: '',
})

afterEach(() => {
  cleanup()
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
})

/**
 * The render-path half of the resolver. A pane MOUNTS before any workspace
 * store has been seeded with the chat it holds, so the interesting property
 * is not the lookup (covered in pane-chat-workspace.test.ts) but that it is
 * asked again once the answer exists.
 */
describe('useChatWorkspaceId', () => {
  it('answers null while nothing knows the chat, then the owner once a store is seeded', () => {
    const { result } = renderHook(() => useChatWorkspaceId('c1'))
    expect(result.current).toBeNull()

    act(() => {
      // A workspace mounting and its chats stream landing — the exact sequence
      // a freshly opened workspace goes through under a pane already showing
      // one of its chats.
      getOrCreateWorkspaceStore('ws-a')
        .getState()
        .seedAgentChats([chat('c1', 'ws-a')])
    })

    expect(result.current).toBe('ws-a')
  })

  it('re-resolves when a store registered AFTER the subscription is seeded', () => {
    getOrCreateWorkspaceStore('ws-a')
    const { result } = renderHook(() => useChatWorkspaceId('c1'))
    expect(result.current).toBeNull()

    act(() => {
      // A brand-new registry entry: the subscription has to pick this store up
      // too, or a chat whose workspace mounts later is never resolved at all.
      getOrCreateWorkspaceStore('ws-b')
        .getState()
        .seedAgentChats([chat('c1', 'ws-b')])
    })

    expect(result.current).toBe('ws-b')
  })

  it('is null for a pane holding no chat, and subscribes to nothing', () => {
    const { result } = renderHook(() => useChatWorkspaceId(null))

    expect(result.current).toBeNull()
  })
})
