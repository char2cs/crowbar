import { describe, expect, it } from 'vitest'

import { recentsChatIcon } from '@/components/sidebar/lib/recents-for-project'
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
