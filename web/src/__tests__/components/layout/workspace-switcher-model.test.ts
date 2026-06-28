import { describe, it, expect } from 'vitest'
import { flattenWorkspaces } from '@/components/layout/workspace-switcher-model'
import type { Repo } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

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

const projects: Project[] = [{ id: 'p1', name: 'Rabbyte' } as Project]

const noHome = { isHomeRoute: false, activeProjectId: undefined as string | undefined, projects: [] as Project[] }

describe('flattenWorkspaces', () => {
  it('prepends a home item when active project is known', () => {
    const items = flattenWorkspaces(repos, undefined, false, 'p1', projects)
    expect(items[0]).toMatchObject({ kind: 'home', projectId: 'p1', projectName: 'Rabbyte', isCurrent: false })
  })

  it('marks home item as current when on home route', () => {
    const items = flattenWorkspaces(repos, undefined, true, 'p1', projects)
    expect(items[0]).toMatchObject({ kind: 'home', isCurrent: true })
  })

  it('includes the default workspace first among workspace items', () => {
    const items = flattenWorkspaces(repos, undefined, ...Object.values(noHome) as [boolean, string | undefined, Project[]])
    const wsItems = items.filter((i) => i.kind === 'workspace')
    expect(wsItems[0]).toMatchObject({ kind: 'workspace', branch: 'default' })
  })

  it('flattens all repos including default workspaces', () => {
    const items = flattenWorkspaces(repos, 'ws3', ...Object.values(noHome) as [boolean, string | undefined, Project[]])
    // no home item (no activeProjectId); r1: default + ws1 + ws2 = 3; r2: ws3 = 1 → total 4
    expect(items).toHaveLength(4)
  })

  it('marks the active workspace as current (regular)', () => {
    const items = flattenWorkspaces(repos, 'ws3', ...Object.values(noHome) as [boolean, string | undefined, Project[]])
    const ws3 = items.find((i) => i.kind === 'workspace' && i.wsId === 'ws3')
    expect(ws3?.isCurrent).toBe(true)
    expect(items.filter((i) => i.isCurrent)).toHaveLength(1)
  })

  it('marks the default workspace as current when active', () => {
    const items = flattenWorkspaces(repos, 'ws-default', ...Object.values(noHome) as [boolean, string | undefined, Project[]])
    const def = items.find((i) => i.kind === 'workspace' && i.wsId === 'ws-default')
    expect(def?.isCurrent).toBe(true)
    expect(items.filter((i) => i.isCurrent)).toHaveLength(1)
  })

  it('defaults a missing status to "new"', () => {
    const items = flattenWorkspaces(repos, undefined, ...Object.values(noHome) as [boolean, string | undefined, Project[]])
    const ws2 = items.find((i) => i.kind === 'workspace' && i.wsId === 'ws2')
    expect(ws2 && 'status' in ws2 ? ws2.status : undefined).toBe('new')
  })

  it('omits default workspace entry when repo has no defaultWorkspaceId', () => {
    const items = flattenWorkspaces(repos, undefined, ...Object.values(noHome) as [boolean, string | undefined, Project[]])
    const r2Items = items.filter((i) => i.kind === 'workspace' && i.repoId === 'r2')
    expect(r2Items).toHaveLength(1)
  })
})
