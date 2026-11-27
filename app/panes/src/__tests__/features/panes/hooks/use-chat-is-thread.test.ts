import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useChatIsThread } from '@/features/panes/hooks/use-chat-is-thread'
import { seedChats, writeChat } from '@/__tests__/__fixtures__/agent-chat'
import { resetEntities, seedHomeTree, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import type { Chat, Repo, Workspace } from '@/core/entities/repo-types'
import type { HomeTree } from '@/core/entities/views-from-entities'

function makeHomeTree(overrides: Partial<HomeTree> = {}): HomeTree {
  return { chats: [], folders: [], ...overrides }
}

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

function chatIsThreadIn(
  repos: Repo[],
  chatId: string | null,
  homeTrees: Record<string, HomeTree> = {},
): boolean {
  resetEntities()
  seedRepos(repos)
  for (const [projectId, tree] of Object.entries(homeTrees)) seedHomeTree(projectId, tree)
  return renderHook(() => useChatIsThread(chatId)).result.current
}

describe('useChatIsThread over the tree', () => {
  it('calls a chat that owns no worktree on its parent branch a thread', () => {
    const repos = [
      makeRepo({
        defaultOwningChatId: 'chat-main',
        chats: [
          makeChat({ id: 'chat-main', workspaceId: 'w-home', ownsWorktree: true, title: 'main' }),
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
        chats: [makeChat({ id: 'chat-main', workspaceId: 'w-home', title: 'main' })],
      }),
    ]
    expect(chatIsThreadIn(repos, 'chat-main')).toBe(false)
  })

  it('does not call a chat a thread when it claims its own worktree', () => {
    const repos = [
      makeRepo({
        workspaces: [makeWorkspace({ id: 'w-fork' })],
        chats: [makeChat({ id: 'chat-fork', workspaceId: 'w-fork', ownsWorktree: true })],
      }),
    ]
    expect(chatIsThreadIn(repos, 'chat-fork')).toBe(false)
  })

  it('does not call a chat a thread when a Workspace names it as its owner', () => {
    const repos = [
      makeRepo({
        workspaces: [makeWorkspace({ id: 'w-fork', owningChatId: 'chat-fork' })],
        chats: [makeChat({ id: 'chat-fork', workspaceId: 'w-fork' })],
      }),
    ]
    expect(chatIsThreadIn(repos, 'chat-fork')).toBe(false)
  })

  it('answers false for a chat no repo in the tree can name yet', () => {
    expect(chatIsThreadIn([makeRepo({ chats: undefined })], 'chat-1')).toBe(false)
    expect(chatIsThreadIn([], 'chat-1')).toBe(false)
  })

  it('answers false for no chat at all', () => {
    expect(chatIsThreadIn([makeRepo({ chats: [] })], null)).toBe(false)
  })

  it('calls a chat found only in a project home tree a thread', () => {
    const homeTrees = {
      'proj-1': makeHomeTree({ chats: [makeChat({ id: 'chat-home', repoId: '' })] }),
    }
    expect(chatIsThreadIn([], 'chat-home', homeTrees)).toBe(true)
    expect(chatIsThreadIn([makeRepo({ chats: [] })], 'chat-home', homeTrees)).toBe(true)
  })

  it('still answers false for a chat named by neither a repo nor a home tree', () => {
    const homeTrees = { 'proj-1': makeHomeTree({ chats: [makeChat({ id: 'chat-home' })] }) }
    expect(chatIsThreadIn([makeRepo({ chats: undefined })], 'chat-1', homeTrees)).toBe(false)
    expect(chatIsThreadIn([], 'chat-1', {})).toBe(false)
  })
})

describe('useChatIsThread', () => {
  it('renders nothing for a change to a chat that leaves the answer as it was', () => {
    seedChats([{ id: 'c1' }, { id: 'c2' }])
    let renders = 0
    renderHook(() => {
      renders += 1
      return useChatIsThread('c1')
    })
    const before = renders
    act(() => writeChat({ id: 'c2', title: 'renamed' }))
    act(() => writeChat({ id: 'c1', title: 'renamed too' }))
    expect(renders).toBe(before)
  })
})
