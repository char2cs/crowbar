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
    const id = s.getState().bufferActions.openContent({
      type: 'agentChat',
      chatId: 'c1',
      wsId: 'w1',
      name: 'Chat 1',
      runnerId: 'r1',
    })
    const buf = s.getState().bufferActions.getBufferById(id) as AgentChatContent
    expect(buf.type).toBe('agentChat')
    expect(buf.chatId).toBe('c1')
    expect(buf.runnerId).toBe('r1')
    expect(buf.wsId).toBe('w1')
    expect(buf.name).toBe('Chat 1')
    expect(buf.path).toBe('agent-chat://c1')
    expect(buf.isPinned).toBe(false)
    expect(buf.isPreview).toBe(false)
    expect(buf.isActive).toBe(false)
  })

  it('defaults runnerId to empty when the opener does not know one', () => {
    // Opening a chat never REQUIRES knowing its runner: the pane adopts whichever
    // runner the chat actually has (and a dormant chat genuinely has none).
    const s = createWorkspaceStore('w1')
    const id = s
      .getState()
      .bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })
    expect((s.getState().bufferActions.getBufferById(id) as AgentChatContent).runnerId).toBe('')
  })

  // ── repointAgentChatBuffer: the tab is a viewport on a moving target ───────
  describe('repointAgentChatBuffer', () => {
    const open = (s: ReturnType<typeof createWorkspaceStore>) =>
      s.getState().bufferActions.openContent({
        type: 'agentChat',
        chatId: 'c1',
        wsId: 'w1',
        name: 'Chat 1',
        runnerId: 'r1',
      })

    it('follows the runner to a new chat: chatId and path move, the runner does not', () => {
      // The user typed /clear inside the CLI: the SAME process is now on a new chat.
      const s = createWorkspaceStore('w1')
      const id = open(s)

      s.getState().bufferActions.repointAgentChatBuffer(id, { chatId: 'c2', runnerId: 'r1' })

      const buf = s.getState().bufferActions.getBufferById(id) as AgentChatContent
      expect(buf.chatId).toBe('c2')
      expect(buf.runnerId).toBe('r1')
      // path is derived from chatId — it must never contradict it.
      expect(buf.path).toBe('agent-chat://c2')
    })

    it('adopts a new runner on the same chat (a Resume, or a provider switch)', () => {
      const s = createWorkspaceStore('w1')
      const id = open(s)

      s.getState().bufferActions.repointAgentChatBuffer(id, { chatId: 'c1', runnerId: 'r2' })

      const buf = s.getState().bufferActions.getBufferById(id) as AgentChatContent
      expect(buf.chatId).toBe('c1')
      expect(buf.runnerId).toBe('r2')
      expect(buf.path).toBe('agent-chat://c1')
    })

    it('lets go of a dead runner (empty runnerId) without dropping the chat', () => {
      const s = createWorkspaceStore('w1')
      const id = open(s)

      s.getState().bufferActions.repointAgentChatBuffer(id, { chatId: 'c1', runnerId: '' })

      const buf = s.getState().bufferActions.getBufferById(id) as AgentChatContent
      expect(buf.chatId).toBe('c1')
      expect(buf.runnerId).toBe('')
    })

    it('leaves the tab LABEL alone (the title effect owns it)', () => {
      const s = createWorkspaceStore('w1')
      const id = open(s)

      s.getState().bufferActions.repointAgentChatBuffer(id, { chatId: 'c2', runnerId: 'r1' })

      expect(s.getState().bufferActions.getBufferById(id)?.name).toBe('Chat 1')
    })

    it('is a no-op when nothing changed — no new buffers array', () => {
      // The pane re-asserts the same pair on every render pass that resolves to it;
      // minting a new array each time would churn every buffers subscriber.
      const s = createWorkspaceStore('w1')
      const id = open(s)
      const before = s.getState().buffers

      s.getState().bufferActions.repointAgentChatBuffer(id, { chatId: 'c1', runnerId: 'r1' })

      expect(s.getState().buffers).toBe(before)
    })

    it('is a no-op for an unknown buffer id and for a non-agentChat buffer', () => {
      const s = createWorkspaceStore('w1')
      const id = open(s)
      const editorId = s
        .getState()
        .bufferActions.openContent({ type: 'editor', path: '/a.ts', name: 'a.ts', content: '' })

      s.getState().bufferActions.repointAgentChatBuffer('nope', { chatId: 'c9', runnerId: 'r9' })
      s.getState().bufferActions.repointAgentChatBuffer(editorId, { chatId: 'c9', runnerId: 'r9' })

      expect((s.getState().bufferActions.getBufferById(id) as AgentChatContent).chatId).toBe('c1')
      expect(s.getState().bufferActions.getBufferById(editorId)?.type).toBe('editor')
      expect(s.getState().bufferActions.getBufferById(editorId)?.path).toBe('/a.ts')
    })
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

  it('renameBuffer relabels the open tab (agent auto-title / user rename)', () => {
    const s = createWorkspaceStore('w1')
    const id = s.getState().bufferActions.openContent({
      type: 'agentChat',
      chatId: 'c1',
      wsId: 'w1',
      name: 'Codex chat',
    })

    s.getState().bufferActions.renameBuffer(id, 'Fix the flaky test')

    const buf = s.getState().bufferActions.getBufferById(id) as AgentChatContent
    expect(buf.name).toBe('Fix the flaky test')
    // Identity fields are untouched — only the label changes.
    expect(buf.chatId).toBe('c1')
    expect(buf.path).toBe('agent-chat://c1')
  })

  it('renameBuffer is a no-op for an unknown buffer id', () => {
    const s = createWorkspaceStore('w1')
    const id = s
      .getState()
      .bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })

    s.getState().bufferActions.renameBuffer('nope', 'Other')

    expect(s.getState().bufferActions.getBufferById(id)?.name).toBe('Chat 1')
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
