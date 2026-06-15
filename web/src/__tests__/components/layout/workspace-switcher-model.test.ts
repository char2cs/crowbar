import { describe, it, expect } from 'vitest'
import { flattenWorkspaces } from '@/components/layout/workspace-switcher-model'
import type { Repo } from '@/lib/store/sidebar'

const repos: Repo[] = [
  {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
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
  it('flattens all repos into items with repo context', () => {
    const items = flattenWorkspaces(repos, 'ws3')
    expect(items).toHaveLength(3)
    expect(items[0]).toEqual({
      wsId: 'ws1',
      repoName: 'crowbar',
      branch: 'develop',
      status: 'pr-open',
      added: 1234,
      deleted: 5,
      isCurrent: false,
    })
  })

  it('marks the active workspace as current', () => {
    const items = flattenWorkspaces(repos, 'ws3')
    expect(items.find((i) => i.wsId === 'ws3')?.isCurrent).toBe(true)
    expect(items.filter((i) => i.isCurrent)).toHaveLength(1)
  })

  it('defaults a missing status to "new"', () => {
    const items = flattenWorkspaces(repos, undefined)
    expect(items.find((i) => i.wsId === 'ws2')?.status).toBe('new')
  })
})
