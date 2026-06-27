import { describe, it, expect } from 'vitest'
import { flattenWorkspaces } from '@/components/layout/workspace-switcher-model'
import type { Repo } from '@/lib/store/sidebar'

const repos: Repo[] = [
  {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    defaultWorkspaceId: 'ws-default',
    workspaces: [
      { id: 'ws1', branch: 'develop', status: 'pr-open', added: 1234, deleted: 5, age: '1d' },
      { id: 'ws2', branch: 'no-status', age: '2d' },
    ],
  },
  {
    id: 'r2',
    name: 'quiver.desktop',
    avatarLabel: 'Q',
    avatarColor: 'bg-teal-700',
    workspaces: [{ id: 'ws3', branch: 'feature/quiver-shell', status: 'new', age: '3d' }],
  },
]

describe('flattenWorkspaces', () => {
  it('includes the default workspace first, before regular workspaces', () => {
    const items = flattenWorkspaces(repos, undefined)
    expect(items[0]).toEqual({
      wsId: 'ws-default',
      projectId: '',
      repoId: 'r1',
      repoName: 'crowbar',
      branch: 'default',
      status: 'new',
      isCurrent: false,
    })
  })

  it('flattens all repos including default workspaces', () => {
    const items = flattenWorkspaces(repos, 'ws3')
    // r1: default + ws1 + ws2 = 3; r2: ws3 = 1 → total 4
    expect(items).toHaveLength(4)
  })

  it('marks the active workspace as current (regular)', () => {
    const items = flattenWorkspaces(repos, 'ws3')
    expect(items.find((i) => i.wsId === 'ws3')?.isCurrent).toBe(true)
    expect(items.filter((i) => i.isCurrent)).toHaveLength(1)
  })

  it('marks the default workspace as current when active', () => {
    const items = flattenWorkspaces(repos, 'ws-default')
    expect(items.find((i) => i.wsId === 'ws-default')?.isCurrent).toBe(true)
    expect(items.filter((i) => i.isCurrent)).toHaveLength(1)
  })

  it('defaults a missing status to "new"', () => {
    const items = flattenWorkspaces(repos, undefined)
    expect(items.find((i) => i.wsId === 'ws2')?.status).toBe('new')
  })

  it('omits default workspace entry when repo has no defaultWorkspaceId', () => {
    const items = flattenWorkspaces(repos, undefined)
    // r2 has no defaultWorkspaceId — only ws3 from r2
    expect(items.filter((i) => i.repoId === 'r2')).toHaveLength(1)
  })
})
