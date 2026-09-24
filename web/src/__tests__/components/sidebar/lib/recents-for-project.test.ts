import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId: (projectId: string) => (projectId === 'p1' ? 'home-ws' : null),
  getHomeOwningChatId: () => null,
}))

import {
  recentsChatIcon,
  recentsChatWorkspaceId,
} from '@/components/sidebar/lib/recents-for-project'
import type { HomeTree } from '@/lib/store/home-tree'
import type { Repo } from '@/lib/store/sidebar'

const repos: Repo[] = [
  {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    defaultWorkspaceId: 'repo-home',
    workspaces: [{ id: 'ws-1', branch: 'feature', age: '', owningChatId: 'chat-owner' }],
    chats: [
      { id: 'chat-repo', repoId: 'r1', title: 'Repo', order: 0, workspaceId: 'ws-1' },
      { id: 'chat-owner', repoId: 'r1', title: 'feature', order: 1, workspaceId: 'ws-1' },
    ],
  },
]
const homeTrees: Record<string, HomeTree> = {
  p1: {
    chats: [{ id: 'chat-home', repoId: '', title: 'Home', order: 0, workspaceId: '' }],
    folders: [],
  },
}

describe('recentsChatWorkspaceId', () => {
  beforeEach(() => vi.clearAllMocks())

  it("answers a repo chat from the sidebar's own chat list — no store needed", () => {
    expect(recentsChatWorkspaceId(repos, homeTrees, 'p1', 'chat-repo')).toBe('ws-1')
  })

  it("answers a project-home chat with the project's home workspace", () => {
    expect(recentsChatWorkspaceId(repos, homeTrees, 'p1', 'chat-home')).toBe('home-ws')
  })

  it("never answers from another project's repos", () => {
    expect(recentsChatWorkspaceId(repos, homeTrees, 'p2', 'chat-repo')).toBe('')
  })
})

describe('recentsChatIcon', () => {
  it("gives a workspace-owning chat the tree's branch glyph fields, shared per repos snapshot", () => {
    const icon = recentsChatIcon(repos, 'chat-owner')
    expect(icon).toMatchObject({ kind: 'branch', ownsWorktree: true, branchName: 'feature' })
    expect(recentsChatIcon(repos, 'chat-owner')).toBe(icon)
  })

  it('leaves a plain chat with no override', () => {
    expect(recentsChatIcon(repos, 'chat-repo')).toBeUndefined()
  })
})
