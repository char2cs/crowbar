import { beforeEach, describe, expect, it } from 'vitest'
import { workspaceRootPath } from '@/lib/workspace/resolve-root-path'
import type { Repo } from '@/core/entities/repo-types'
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { seedProjects, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

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
  seedRepos([repo])
  __resetWorkspaceScopesForTest()
  // The workspace in view is never the one asked about.
  setActiveWorkspaceId('ws-in-view')
})

describe('workspaceRootPath', () => {
  it('returns the worktree path of the workspace it is given', () => {
    expect(workspaceRootPath('ws-feature')).toBe('/disk/worktrees/feat')
  })

  it('falls back to the repo root when the workspace has no localPath', () => {
    expect(workspaceRootPath('ws-no-path')).toBe('/disk/repo-root')
  })

  it('maps the default (main-worktree) workspace to the repo root', () => {
    expect(workspaceRootPath('ws-default')).toBe('/disk/repo-root')
  })

  it("maps a project's home workspace, which no repo lists, to the project's own path", () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'ws-home' })
    seedProjects([{ id: 'p1', path: '/disk/project' }] as never)
    expect(workspaceRootPath('ws-home')).toBe('/disk/project')
  })

  it('returns undefined for an unknown workspace', () => {
    expect(workspaceRootPath('ws-unknown')).toBeUndefined()
  })
})
