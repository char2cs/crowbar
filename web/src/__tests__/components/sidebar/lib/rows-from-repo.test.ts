import { describe, expect, it } from 'vitest'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import type { Folder, Repo, Workspace } from '@/lib/store/sidebar'

function makeTestWorkspace(over: Partial<Workspace> & { id: string; branch: string }): Workspace {
  return { age: '', ...over }
}

function makeTestFolder(over: Partial<Folder> & { id: string; name: string }): Folder {
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

describe('rowsFromRepo', () => {
  it('a locked branch becomes a branch-kind row', () => {
    const repo = makeTestRepo({
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'develop', status: 'locked' })],
    })
    const rows = rowsFromRepo(repo)
    const row = rows.find((r) => r.workspaceId === 'ws-1')
    expect(row?.kind).toBe('branch')
    expect(row?.ownsWorktree).toBe(true)
  })

  it('a chat folder becomes a folder-kind row', () => {
    const repo = makeTestRepo({
      folders: [makeTestFolder({ id: 'f-1', name: 'Bugs' })],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.id === 'f-1')?.kind).toBe('folder')
  })

  it('the default workspace becomes the one root row, labelled with the repo name', () => {
    const repo = makeTestRepo({ defaultWorkspaceId: 'ws-home', defaultBranch: 'main' })
    const rows = rowsFromRepo(repo)
    const home = rows.find((r) => r.id === 'ws-home')
    expect(home?.kind).toBe('branch')
    expect(home?.parentId).toBeNull()
    expect(home?.label).toBe('crowbar')
    expect(home?.branchName).toBe('main')
  })

  it('a root-level workspace nests under the default workspace', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' })],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.id === 'ws-1')?.parentId).toBe('ws-home')
  })

  it('a forked workspace nests under its fork parent, not the default workspace', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [
        makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' }),
        makeTestWorkspace({ id: 'ws-2', branch: 'feature/x/child', parentId: 'ws-1' }),
      ],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.id === 'ws-2')?.parentId).toBe('ws-1')
  })

  it('drops a workspace whose status is a deleted tombstone', () => {
    const repo = makeTestRepo({
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'gone', status: 'deleted' })],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.workspaceId === 'ws-1')).toBeUndefined()
  })

  it('produces no rows for a repo with nothing yet', () => {
    expect(rowsFromRepo(makeTestRepo())).toEqual([])
  })
})
