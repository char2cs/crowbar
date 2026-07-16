import { describe, expect, it } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { AgentChatContent } from '@/features/panes/types/pane-content'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

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

  // ── Re-opening a chat REVEALS its existing view, never duplicates it ───────
  // An agent chat is a live view onto ONE PTY. Dropping a second copy into a
  // different pane races the first over the shared transport and one of the two
  // goes blank (the duplication-blank bug). Re-open must jump to the pane that
  // already holds it — not add a copy to the active pane.
  describe('openContent reveals an existing agentChat instead of duplicating it', () => {
    it('re-opening a chat held in pane P while pane Q is active jumps focus to P, and Q gets no copy', () => {
      const s = createWorkspaceStore('w1')
      const paneP = s.getState().activePaneId
      expect(paneP).toBe(ROOT_PANE_ID)

      // The chat lives in pane P.
      const idX = s
        .getState()
        .bufferActions.openContent({ type: 'agentChat', chatId: 'cX', wsId: 'w1', name: 'Chat X' })

      // Split off a second pane Q and make it the active one, with its own buffer.
      const paneQ = s.getState().paneActions.splitPane(paneP, 'vertical')
      expect(paneQ).not.toBeNull()
      s.getState().bufferActions.openContent({
        type: 'editor',
        path: '/other.ts',
        name: 'other.ts',
        content: '',
      })
      expect(s.getState().activePaneId).toBe(paneQ)

      // Re-open the SAME chat from Q.
      const idAgain = s
        .getState()
        .bufferActions.openContent({ type: 'agentChat', chatId: 'cX', wsId: 'w1', name: 'Chat X' })

      // Same buffer, revealed in place.
      expect(idAgain).toBe(idX)
      // Focus jumped to the pane that already holds it...
      expect(s.getState().activePaneId).toBe(paneP)
      expect(s.getState().panes[paneP]?.activeBufferId).toBe(idX)
      // ...and NO second pane holds a copy.
      const holders = Object.values(s.getState().panes).filter((p) => p.bufferIds.includes(idX))
      expect(holders).toHaveLength(1)
      expect(holders[0]?.id).toBe(paneP)
      expect(s.getState().panes[paneQ as string]?.bufferIds).not.toContain(idX)
    })

    it('re-opening a terminal (same latent bug) also reveals its pane, never duplicates', () => {
      const s = createWorkspaceStore('w1')
      const paneP = s.getState().activePaneId

      const idT = s
        .getState()
        .bufferActions.openContent({ type: 'terminal', sessionId: 'sess-X', name: 'Terminal X' })

      const paneQ = s.getState().paneActions.splitPane(paneP, 'vertical')
      s.getState().bufferActions.openContent({
        type: 'editor',
        path: '/f.ts',
        name: 'f.ts',
        content: '',
      })
      expect(s.getState().activePaneId).toBe(paneQ)

      const idAgain = s
        .getState()
        .bufferActions.openContent({ type: 'terminal', sessionId: 'sess-X', name: 'Terminal X' })

      expect(idAgain).toBe(idT)
      expect(s.getState().activePaneId).toBe(paneP)
      const holders = Object.values(s.getState().panes).filter((p) => p.bufferIds.includes(idT))
      expect(holders).toHaveLength(1)
      expect(holders[0]?.id).toBe(paneP)
    })

    it('EDITOR dedup is UNCHANGED: re-opening surfaces the file in the ACTIVE pane', () => {
      // Editors are safe to appear in more than one pane, and the long-standing
      // behavior — open the existing file into whatever pane is active — must not
      // change. Only terminals/agent chats are pinned to their holding pane.
      const s = createWorkspaceStore('w1')
      const paneP = s.getState().activePaneId

      const idE = s
        .getState()
        .bufferActions.openContent({ type: 'editor', path: '/dup.ts', name: 'dup.ts', content: '' })

      const paneQ = s.getState().paneActions.splitPane(paneP, 'vertical')
      expect(s.getState().activePaneId).toBe(paneQ)

      const idAgain = s
        .getState()
        .bufferActions.openContent({ type: 'editor', path: '/dup.ts', name: 'dup.ts', content: '' })

      expect(idAgain).toBe(idE)
      // Surfaced into the ACTIVE pane Q (editors may live in multiple panes).
      expect(s.getState().activePaneId).toBe(paneQ)
      expect(s.getState().panes[paneQ as string]?.bufferIds).toContain(idE)
      expect(s.getState().panes[paneQ as string]?.activeBufferId).toBe(idE)
    })
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
