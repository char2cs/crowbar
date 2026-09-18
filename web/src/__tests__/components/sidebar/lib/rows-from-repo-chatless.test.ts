import { describe, expect, it } from 'vitest'

import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import type { Chat, Repo } from '@/lib/store/sidebar'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'repo-alpha',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-main',
  defaultBranch: 'main',
  defaultOwningChatId: '',
  workspaces: [],
  folders: [],
  chats: [],
  ...over,
})

const lockedChatless = {
  id: 'ws-locked',
  branch: 'release',
  age: '',
  order: 0,
  status: 'locked' as const,
  owningChatId: '',
}

describe('rows-from-repo — chatless workspaces', () => {
  it('ids a chatless home row and a chatless locked branch by their WORKSPACE id', () => {
    const rows = rowsFromRepo(repo({ workspaces: [lockedChatless] }))
    expect(rows.map((r) => [r.id, r.kind, r.workspaceId])).toEqual([
      ['ws-main', 'branch', 'ws-main'],
      ['ws-locked', 'branch', 'ws-locked'],
    ])
  })

  it('draws a thread started inside a chatless locked branch as its own row while the daemon still reports no owner', () => {
    const thread: Chat = {
      id: 't1',
      repoId: 'r1',
      title: '',
      order: 0,
      parentId: 'ws-locked',
      workspaceId: 'ws-locked',
      ownsWorktree: false,
    }
    const rows = rowsFromRepo(repo({ workspaces: [lockedChatless], chats: [thread] }))
    expect(rows.map((r) => [r.id, r.kind, r.parentId])).toEqual([
      ['ws-main', 'branch', null],
      ['ws-locked', 'branch', 'ws-main'],
      ['t1', 'chat', 'ws-locked'],
    ])
  })

  // REGRESSION: the daemon used to promote the only chat of a chatless
  // workspace — the thread just started inside it — to the workspace owner,
  // and the thread row vanished into the branch row. Ownership is a recorded
  // fact now (the owner is minted on first read, ResolveOwningChat never
  // picks a thread filed under the workspace), so the wire names a separate
  // owner and the thread stays its own row.
  it('a thread inside a locked branch stays its own row under the recorded owner', () => {
    const owner: Chat = {
      id: 'o1',
      repoId: 'r1',
      title: '',
      order: 0,
      parentId: 'ws-main',
      workspaceId: 'ws-locked',
      ownsWorktree: true,
    }
    const thread: Chat = {
      id: 't1',
      repoId: 'r1',
      title: 'fix the build',
      order: 0,
      parentId: 'o1',
      workspaceId: 'ws-locked',
      ownsWorktree: false,
    }
    const rows = rowsFromRepo(
      repo({ workspaces: [{ ...lockedChatless, owningChatId: 'o1' }], chats: [owner, thread] }),
    )
    expect(rows.map((r) => [r.id, r.kind, r.parentId, r.locked])).toEqual([
      ['ws-main', 'branch', null, false],
      ['o1', 'branch', 'ws-main', true],
      ['t1', 'chat', 'o1', undefined],
    ])
  })

  it('a thread on the repo home stays its own row under the recorded owner', () => {
    const owner: Chat = {
      id: 'o1',
      repoId: 'r1',
      title: '',
      order: 0,
      workspaceId: 'ws-main',
      ownsWorktree: true,
    }
    const thread: Chat = {
      id: 't1',
      repoId: 'r1',
      title: 'fix the build',
      order: 0,
      parentId: 'o1',
      workspaceId: 'ws-main',
      ownsWorktree: false,
    }
    const rows = rowsFromRepo(repo({ defaultOwningChatId: 'o1', chats: [owner, thread] }))
    expect(rows.map((r) => [r.id, r.kind, r.label, r.parentId])).toEqual([
      ['o1', 'branch', 'repo-alpha', null],
      ['t1', 'chat', 'fix the build', 'o1'],
    ])
  })
})
