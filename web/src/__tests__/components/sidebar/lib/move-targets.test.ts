import { beforeEach, describe, expect, it } from 'vitest'
import { moveTargets } from '@/components/sidebar/lib/move-targets'
import type { Repo } from '@/core/entities/repo-types'
import { repoRows } from '@/__tests__/__fixtures__/tree-rows'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

const REPO: Repo = {
  id: 'repo-1',
  projectId: 'proj-1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-home',
  defaultBranch: 'main',
  workspaces: [{ id: 'ws-1', branch: 'feature-x', age: '' }],
  folders: [
    { id: 'bugs', repoId: 'repo-1', name: 'Bugs', order: 0 },
    { id: 'old-bugs', repoId: 'repo-1', name: 'Old bugs', order: 0, parentId: 'bugs' },
  ],
  chats: [
    {
      id: 'home-row',
      repoId: 'repo-1',
      ownsWorktree: true,
      workspaceId: 'ws-home',
      title: '',
      order: 0,
    },
    {
      id: 'thread-1',
      repoId: 'repo-1',
      ownsWorktree: false,
      title: 'A thread',
      order: 2,
      parentId: 'bugs',
    },
  ],
}

const labels = (rowId: string) => {
  const rows = repoRows(REPO)
  const row = rows.find((r) => r.id === rowId)!
  return moveTargets(row, rows).map((t) => t.label)
}

beforeEach(() => {
  resetEntities()
  seedRepos([REPO])
})

// The menu's Move to… is the drag's keyboard twin: it offers what a drop could do, no more.
describe('moveTargets', () => {
  it('offers a thread every other folder, never the folder it is already in', () => {
    expect(labels('thread-1')).toEqual(['Old bugs'])
  })

  it('never offers a folder itself or a folder inside it', () => {
    expect(labels('bugs')).not.toContain('Bugs')
    expect(labels('bugs')).not.toContain('Old bugs')
  })

  it('offers a workspace the folders it could be filed into', () => {
    expect(labels('ws-1')).toEqual(expect.arrayContaining(['Bugs', 'Old bugs']))
    expect(labels('ws-1')).not.toContain('feature-x')
  })
})
