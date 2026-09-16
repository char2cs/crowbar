import { describe, expect, it } from 'vitest'
import {
  owningChatIdOfWorkspace,
  workspaceIdOfBranchRow,
} from '@/components/sidebar/lib/branch-row-id'
import type { Chat, Repo, Workspace } from '@/lib/store/sidebar'

function makeTestWorkspace(over: Partial<Workspace> & { id: string; branch: string }): Workspace {
  return { age: '', ...over }
}

function makeTestChat(over: Partial<Chat> & { id: string; title: string }): Chat {
  return { repoId: 'r1', order: 0, ...over }
}

function makeTestRepo(over: Partial<Repo> = {}): Repo {
  return {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...over,
  }
}

/**
 * 2026-09-08 sidebar-placement-unification Task 9 stops minting/retyping a
 * `'branch'`-typed chat to mark a repo/project home's owning row — these pin
 * that neither direction of the id-space translation depends on `Chat.type`
 * any more, reading `Repo.defaultOwningChatId` (or, absent that, the
 * type-agnostic `ownsWorktree` fallback) instead.
 */
describe('owningChatIdOfWorkspace', () => {
  it('resolves a locked branch directly off Workspace.owningChatId', () => {
    const repo = makeTestRepo({
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'develop', owningChatId: 'chat-1' })],
    })
    expect(owningChatIdOfWorkspace([repo], 'ws-1')).toBe('chat-1')
  })

  it('resolves the repo-home row off Repo.defaultOwningChatId, with no chats at all', () => {
    const repo = makeTestRepo({ defaultWorkspaceId: 'ws-home', defaultOwningChatId: 'home-chat' })
    expect(owningChatIdOfWorkspace([repo], 'ws-home')).toBe('home-chat')
  })

  it('falls back to an ownsWorktree chat for the home row when defaultOwningChatId is absent', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [
        makeTestChat({ id: 'home-chat', title: '', workspaceId: 'ws-home', ownsWorktree: true }),
      ],
    })
    expect(owningChatIdOfWorkspace([repo], 'ws-home')).toBe('home-chat')
  })

  it('never matches a legacy type: "branch"-less chat that merely shares the workspace id', () => {
    // A thread carries its parent's workspaceId without owning it — no
    // ownsWorktree, so it must never stand in as the home row's owner.
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeTestChat({ id: 'unrelated-thread', title: '', workspaceId: 'ws-home' })],
    })
    expect(owningChatIdOfWorkspace([repo], 'ws-home')).toBeNull()
  })

  it('returns null for an unknown workspace id', () => {
    expect(owningChatIdOfWorkspace([makeTestRepo()], 'nope')).toBeNull()
  })
})

describe('workspaceIdOfBranchRow', () => {
  it('resolves a locked branch row id back to its workspace id', () => {
    const repo = makeTestRepo({
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'develop', owningChatId: 'chat-1' })],
    })
    expect(workspaceIdOfBranchRow([repo], 'chat-1')).toBe('ws-1')
  })

  it('resolves the repo-home row id back to defaultWorkspaceId via defaultOwningChatId', () => {
    const repo = makeTestRepo({ defaultWorkspaceId: 'ws-home', defaultOwningChatId: 'home-chat' })
    expect(workspaceIdOfBranchRow([repo], 'home-chat')).toBe('ws-home')
  })

  it('falls back to an ownsWorktree chat for the home row when defaultOwningChatId is absent', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [
        makeTestChat({ id: 'home-chat', title: '', workspaceId: 'ws-home', ownsWorktree: true }),
      ],
    })
    expect(workspaceIdOfBranchRow([repo], 'home-chat')).toBe('ws-home')
  })

  it('returns null for an id that owns nothing', () => {
    expect(workspaceIdOfBranchRow([makeTestRepo()], 'nope')).toBeNull()
  })
})
