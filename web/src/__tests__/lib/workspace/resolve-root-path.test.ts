import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resolveWorkspaceRootPath, workspaceRootPath } from '@/lib/workspace/resolve-root-path'
import type { Repo } from '@/core/entities/repo-types'
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import { seedProjects, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

const { getActiveWorkspaceId } = vi.hoisted(() => ({
  getActiveWorkspaceId: vi.fn<() => string | null>(() => null),
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId,
}))

const repo: Repo = {
  id: 'r1',
  name: 'demo-repo',
  avatarLabel: 'A',
  avatarColor: 'avatar-rose',
  defaultWorkspaceId: 'ws-default',
  localPath: '/disk/repo-root',
  workspaces: [
    {
      id: 'ws-feature',
      branch: 'feat',
      localPath: '/disk/worktrees/feat',
    } as Repo['workspaces'][0],
    { id: 'ws-no-path', branch: 'stub' } as Repo['workspaces'][0],
  ],
}

beforeEach(() => {
  vi.clearAllMocks()
  seedRepos([repo])
  __resetWorkspaceScopesForTest()
  window.location.hash = ''
})

describe('resolveWorkspaceRootPath', () => {
  it('returns the workspace worktree path for a listed workspace', () => {
    getActiveWorkspaceId.mockReturnValue('ws-feature')
    expect(resolveWorkspaceRootPath()).toBe('/disk/worktrees/feat')
  })

  it('falls back to the repo root when the workspace has no localPath', () => {
    getActiveWorkspaceId.mockReturnValue('ws-no-path')
    expect(resolveWorkspaceRootPath()).toBe('/disk/repo-root')
  })

  it('maps the default (main-worktree) workspace to the repo root', () => {
    getActiveWorkspaceId.mockReturnValue('ws-default')
    expect(resolveWorkspaceRootPath()).toBe('/disk/repo-root')
  })

  it('returns undefined for an unknown workspace off the home route', () => {
    getActiveWorkspaceId.mockReturnValue('ws-unknown')
    expect(resolveWorkspaceRootPath()).toBeUndefined()
  })
})

describe('workspaceRootPath', () => {
  it('answers for the workspace it is given, whichever is active', () => {
    getActiveWorkspaceId.mockReturnValue('ws-default')
    expect(workspaceRootPath('ws-feature')).toBe('/disk/worktrees/feat')
  })

  it("maps a project's home workspace to the project's own path", () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'ws-home' })
    seedProjects([{ id: 'p1', path: '/disk/project' }] as never)
    expect(workspaceRootPath('ws-home')).toBe('/disk/project')
  })
})
