import { describe, it, expect } from 'vitest'
import { deriveContextPillModel } from '@/components/layout/context-pill-model'
import type { Repo } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

const repos: Repo[] = [
  {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws1', branch: 'ide-polish', status: 'pr-open', age: '1d' },
      { id: 'ws2', branch: 'no-status', age: '2d' },
    ],
  },
]

const projects: Project[] = [{ id: 'p1', name: 'Crowbar', path: '/x', lastActivity: new Date(0) }]

describe('deriveContextPillModel', () => {
  it('returns workspace model when the active workspace resolves', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: 'ws1',
      isHomeRoute: false,
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toEqual({
      kind: 'workspace',
      status: 'pr-open',
      repoName: 'crowbar',
      branchName: 'ide-polish',
    })
  })

  it('falls back to status "new" when the workspace has no status', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: 'ws2',
      isHomeRoute: false,
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toMatchObject({ kind: 'workspace', status: 'new', branchName: 'no-status' })
  })

  it('returns project model when no workspace is active', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: undefined,
      isHomeRoute: false,
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toEqual({ kind: 'project', projectName: 'Crowbar' })
  })

  it('labels the default workspace "default" with the repo name', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: 'ws-default',
      isHomeRoute: false,
      repos: [{ ...repos[0], defaultWorkspaceId: 'ws-default' }],
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toEqual({
      kind: 'workspace',
      status: 'new',
      repoName: 'crowbar',
      branchName: 'default',
      // The default (imported-folder) workspace shows the repo avatar in the pill.
      repoAvatar: { url: undefined, label: 'C', color: 'bg-indigo-700' },
    })
  })

  it('returns project model when the workspace id does not resolve', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: 'missing',
      isHomeRoute: false,
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toEqual({ kind: 'project', projectName: 'Crowbar' })
  })

  it('returns empty when nothing resolves', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: undefined,
      isHomeRoute: false,
      repos,
      projects,
      activeProjectId: '',
    })
    expect(model).toEqual({ kind: 'empty' })
  })
})
