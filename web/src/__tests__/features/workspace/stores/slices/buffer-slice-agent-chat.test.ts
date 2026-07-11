import { describe, expect, it } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { AgentChatContent } from '@/features/panes/types/pane-content'

describe('buffer-slice agentChat', () => {
  it('opens an agentChat buffer and dedups/focuses by chatId', () => {
    const s = createWorkspaceStore('w1')
    const id1 = s
      .getState()
      .bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })
    const id2 = s
      .getState()
      .bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })
    expect(id2).toBe(id1) // same chatId → existing buffer focused, not duplicated
    expect(s.getState().buffers).toHaveLength(1)
    const buf = s.getState().bufferActions.getBufferById(id1)
    expect(buf?.type).toBe('agentChat')
    expect((buf as { chatId?: string }).chatId).toBe('c1')
  })

  it('builds the agentChat buffer with the expected fields', () => {
    const s = createWorkspaceStore('w1')
    const id = s
      .getState()
      .bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })
    const buf = s.getState().bufferActions.getBufferById(id) as AgentChatContent
    expect(buf.type).toBe('agentChat')
    expect(buf.chatId).toBe('c1')
    expect(buf.wsId).toBe('w1')
    expect(buf.name).toBe('Chat 1')
    expect(buf.path).toBe('agent-chat://c1')
    expect(buf.isPinned).toBe(false)
    expect(buf.isPreview).toBe(false)
    expect(buf.isActive).toBe(false)
  })

  it('opening a different chatId creates a separate buffer (no cross-chat dedup)', () => {
    const s = createWorkspaceStore('w1')
    const id1 = s
      .getState()
      .bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })
    const id2 = s
      .getState()
      .bufferActions.openContent({ type: 'agentChat', chatId: 'c2', wsId: 'w1', name: 'Chat 2' })
    expect(id2).not.toBe(id1)
    expect(s.getState().buffers).toHaveLength(2)
  })

  it('reopening an existing agentChat buffer focuses it in the active pane', () => {
    const s = createWorkspaceStore('w1')
    const id1 = s
      .getState()
      .bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })
    // Open something else so the agentChat tab is no longer the active buffer.
    s.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
    })
    const activePaneId = s.getState().activePaneId
    expect(s.getState().panes[activePaneId]?.activeBufferId).not.toBe(id1)

    const id2 = s
      .getState()
      .bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })
    expect(id2).toBe(id1)
    expect(s.getState().panes[activePaneId]?.activeBufferId).toBe(id1)
  })
})
