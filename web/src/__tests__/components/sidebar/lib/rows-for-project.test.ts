import { describe, expect, it } from 'vitest'
import { rowsForProject } from '@/components/sidebar/lib/rows-for-project'
import type { Repo, Workspace } from '@/lib/store/sidebar'

function makeTestWorkspace(over: Partial<Workspace> & { id: string; branch: string }): Workspace {
  return { age: '', ...over }
}

function makeTestRepo(over: Partial<Repo> = {}): Repo {
  return {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...over,
  }
}

describe('rowsForProject', () => {
  it("returns only the given project's rows", () => {
    const repos = [
      makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        defaultWorkspaceId: 'home-1',
        workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'alpha' })],
      }),
      makeTestRepo({
        id: 'r2',
        projectId: 'p2',
        defaultWorkspaceId: 'home-2',
        workspaces: [makeTestWorkspace({ id: 'ws-2', branch: 'beta' })],
      }),
    ]

    const p1Rows = rowsForProject(repos, 'p1')
    expect(p1Rows.map((r) => r.id)).toEqual(['home-1', 'ws-1'])
    expect(p1Rows.some((r) => r.id === 'home-2' || r.id === 'ws-2')).toBe(false)

    const p2Rows = rowsForProject(repos, 'p2')
    expect(p2Rows.map((r) => r.id)).toEqual(['home-2', 'ws-2'])
  })

  it('a repo with no projectId yet belongs to no space', () => {
    const repos = [makeTestRepo({ id: 'r1', projectId: undefined })]
    expect(rowsForProject(repos, 'p1')).toEqual([])
  })

  it('an unknown projectId yields no rows', () => {
    const repos = [makeTestRepo({ id: 'r1', projectId: 'p1' })]
    expect(rowsForProject(repos, 'does-not-exist')).toEqual([])
  })

  it('two repos under the SAME project both contribute rows', () => {
    const repos = [
      makeTestRepo({ id: 'r1', projectId: 'p1', defaultWorkspaceId: 'home-1' }),
      makeTestRepo({ id: 'r2', projectId: 'p1', defaultWorkspaceId: 'home-2' }),
    ]
    expect(
      rowsForProject(repos, 'p1')
        .map((r) => r.id)
        .sort(),
    ).toEqual(['home-1', 'home-2'])
  })
})
