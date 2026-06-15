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

const projects: Project[] = [
  { id: 'p1', name: 'Crowbar', path: '/x', lastActivity: new Date(0) },
]

describe('deriveContextPillModel', () => {
  it('returns workspace model when the active workspace resolves', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: 'ws1',
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
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toMatchObject({ kind: 'workspace', status: 'new', branchName: 'no-status' })
  })

  it('returns project model when no workspace is active', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: undefined,
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toEqual({ kind: 'project', projectName: 'Crowbar' })
  })

  it('returns project model when the workspace id does not resolve', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: 'missing',
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toEqual({ kind: 'project', projectName: 'Crowbar' })
  })

  it('returns empty when nothing resolves', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: undefined,
      repos,
      projects,
      activeProjectId: '',
    })
    expect(model).toEqual({ kind: 'empty' })
  })
})
