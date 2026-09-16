import { describe, expect, it } from 'vitest'
import { chatIsThreadIn } from '@/features/panes/hooks/use-chat-is-thread'
import type { Chat, Repo, Workspace } from '@/lib/store/sidebar'

function makeChat(overrides: Partial<Chat> = {}): Chat {
  return {
    id: 'chat-1',
    repoId: 'repo-1',
    title: 'Hi claude',
    order: 0,
    ...overrides,
  }
}

function makeWorkspace(overrides: Partial<Workspace> = {}): Workspace {
  return { id: 'w-fork', branch: 'feature/x', age: '1m', ...overrides } as Workspace
}

function makeRepo(overrides: Partial<Repo> = {}): Repo {
  return {
    id: 'repo-1',
    projectId: 'proj-1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: '#000',
    workspaces: [],
    defaultWorkspaceId: 'w-home',
    defaultBranch: 'main',
    ...overrides,
  } as Repo
}

// The chat's own worktree OWNERSHIP is the only signal that may gate branch
// chrome — `Chat.workspaceId` cannot, because a thread carries its parent's
// and so names a worktree it does not own. Same authority `rows-from-repo.ts`
// folds a `branch` row from.
describe('chatIsThreadIn', () => {
  it('calls a chat that owns no worktree on its parent branch a thread', () => {
    const repos = [
      makeRepo({
        defaultOwningChatId: 'chat-main',
        chats: [
          makeChat({ id: 'chat-main', workspaceId: 'w-home', ownsWorktree: true, title: 'main' }),
          // Nested under the branch's own chat, running on its ground.
          makeChat({
            id: 'chat-1',
            workspaceId: 'w-home',
            ownsWorktree: false,
            parentId: 'chat-main',
          }),
        ],
      }),
    ]
    expect(chatIsThreadIn(repos, 'chat-1')).toBe(true)
  })

  it("does not call the repo home's own owning chat a thread", () => {
    const repos = [
      makeRepo({
        defaultOwningChatId: 'chat-main',
        // Deliberately no `ownsWorktree` on the row: the repo home is never a
        // member of `repo.workspaces`, so `Repo.defaultOwningChatId` is the
        // only join available for a row cached before the field existed.
        chats: [makeChat({ id: 'chat-main', workspaceId: 'w-home', title: 'main' })],
      }),
    ]
    expect(chatIsThreadIn(repos, 'chat-main')).toBe(false)
  })

  it('does not call a chat a thread when it claims its own worktree', () => {
    const repos = [
      makeRepo({
        chats: [makeChat({ id: 'chat-fork', workspaceId: 'w-fork', ownsWorktree: true })],
      }),
    ]
    expect(chatIsThreadIn(repos, 'chat-fork')).toBe(false)
  })

  it('does not call a chat a thread when a Workspace names it as its owner', () => {
    const repos = [
      makeRepo({
        workspaces: [makeWorkspace({ id: 'w-fork', owningChatId: 'chat-fork' })],
        // The chat half arrived before its own `ownsWorktree` did.
        chats: [makeChat({ id: 'chat-fork', workspaceId: 'w-fork' })],
      }),
    ]
    expect(chatIsThreadIn(repos, 'chat-fork')).toBe(false)
  })

  // `Repo.chats` is absent while a repo's seed is in flight, and for a folded
  // section that has no subscription open at all — "not yet", never "no".
  it('answers false for a chat no repo in the tree can name yet', () => {
    expect(chatIsThreadIn([makeRepo({ chats: undefined })], 'chat-1')).toBe(false)
    expect(chatIsThreadIn([], 'chat-1')).toBe(false)
  })

  it('answers false for no chat at all', () => {
    expect(chatIsThreadIn([makeRepo({ chats: [] })], null)).toBe(false)
  })
})
