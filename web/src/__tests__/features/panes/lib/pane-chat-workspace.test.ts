import { afterEach, describe, expect, it, vi } from 'vitest'

// Real registry, real workspace stores — the whole point of this resolver is
// which store actually holds a chat. Only the persistence side effects those
// stores fire on creation are stubbed, the same way drop-actions.test.ts does
// it.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { isKnownChatId, resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { AgentChat } from '@/features/agent/api/agent-api'

const chat = (id: string, wsId: string, over: Partial<AgentChat> = {}): AgentChat => ({
  id,
  workspaceId: wsId,
  title: id,
  liveRunnerId: '',
  terminalSessionId: '',
  activeProviderId: 'claude',
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
  parentId: '',
  ...over,
})

/** Seed `wsId`'s store with a chat list, as its own agent-chats stream does. */
function seed(wsId: string, chats: AgentChat[]): void {
  getOrCreateWorkspaceStore(wsId).getState().seedAgentChats(chats)
}

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
})

/**
 * `resolveChatWorkspaceId` — "which workspace does this chat belong to",
 * answered without asking which workspace happens to be routed.
 *
 * The question the pane render path had no way to ask, and the reason a drag
 * from any row outside the active workspace was refused outright.
 */
describe('resolveChatWorkspaceId', () => {
  it('answers from the chat record itself', () => {
    seed('ws-a', [chat('c1', 'ws-a')])

    expect(resolveChatWorkspaceId('c1')).toBe('ws-a')
  })

  it('does NOT answer with the registry key it happened to be found under', () => {
    // `listChats` is REPO-scoped: every workspace store in a repo is seeded
    // with that whole repo's chats, so `ws-b`'s store legitimately holds a
    // chat that belongs to `ws-a`. Returning "where I found it" here is the
    // bug this ordering exists to avoid — it would name ws-b as the owner of
    // every chat in the repo.
    seed('ws-b', [chat('c1', 'ws-a'), chat('c2', 'ws-b')])

    expect(resolveChatWorkspaceId('c1')).toBe('ws-a')
    expect(resolveChatWorkspaceId('c2')).toBe('ws-b')
  })

  it('prefers the chat record over the caller’s hint when the two disagree', () => {
    seed('ws-a', [chat('c1', 'ws-a')])

    expect(resolveChatWorkspaceId('c1', 'ws-stale')).toBe('ws-a')
  })

  it('falls back to the hint for a chat no store has been seeded with yet', () => {
    // The routine case for a workspace the user has never opened: the sidebar
    // knows the row, no workspace store knows the chat.
    expect(resolveChatWorkspaceId('c1', 'ws-a')).toBe('ws-a')
  })

  it('is null when nothing in the app can name a workspace for the chat', () => {
    expect(resolveChatWorkspaceId('nobody-knows-me')).toBeNull()
    expect(resolveChatWorkspaceId('nobody-knows-me', null)).toBeNull()
  })

  it('survives its owning workspace being evicted, via any other store in the repo', () => {
    seed('ws-a', [chat('c1', 'ws-a')])
    seed('ws-b', [chat('c1', 'ws-a'), chat('c2', 'ws-b')]) // repo-scoped seed
    destroyWorkspaceStore('ws-a')

    // A pane holding c1 outlives ws-a's eviction by design (Task 26) — and
    // still resolves the right owner, because the answer rides on the chat.
    expect(resolveChatWorkspaceId('c1')).toBe('ws-a')
  })
})

/**
 * `isKnownChatId` — the CHECK the hint clause deliberately is not.
 *
 * A `branch` row is id'd from the chat that owns its workspace, but falls back
 * to its own WORKSPACE id when that chat cannot be resolved
 * (`rows-from-repo.ts`). Handing that id to a pane would point it at a chat
 * that does not exist, so the pane-drop path asks this before treating a row
 * id as a chat id.
 */
describe('isKnownChatId', () => {
  it('is true only for an id some registered store actually holds as a chat', () => {
    seed('ws-a', [chat('c1', 'ws-a')])

    expect(isKnownChatId('c1')).toBe(true)
    expect(isKnownChatId('ws-a')).toBe(false)
    expect(isKnownChatId('never-existed')).toBe(false)
  })
})
