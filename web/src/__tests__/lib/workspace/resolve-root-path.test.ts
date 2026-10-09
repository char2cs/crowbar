import { beforeEach, describe, expect, it } from 'vitest'
import { resolveWorkspaceRootPath } from '@/lib/workspace/resolve-root-path'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useProjectDataStore } from '@/lib/store/projects'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { loadProjects, projectOn, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

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
  resetHostWorld()
  __resetWorkspaceScopesForTest()
  useSidebarStore.setState({ repos: [repo] })
  useProjectDataStore.setState(useProjectDataStore.getInitialState())
  // The workspace in view is never the one asked about.
  setActiveWorkspaceId('ws-in-view')
})

describe('resolveWorkspaceRootPath', () => {
  it('returns the worktree path of the workspace it is given', () => {
    expect(resolveWorkspaceRootPath('ws-feature')).toBe('/disk/worktrees/feat')
  })

  it('falls back to the repo root when the workspace has no localPath', () => {
    expect(resolveWorkspaceRootPath('ws-no-path')).toBe('/disk/repo-root')
  })

  it('maps the default (main-worktree) workspace to the repo root', () => {
    expect(resolveWorkspaceRootPath('ws-default')).toBe('/disk/repo-root')
  })

  it('maps a project-home workspace, which the sidebar repos do not list, to the project path', () => {
    recordWorkspaceScope({ projectId: 'p-home', repoId: '', wsId: 'ws-home' })
    loadProjects({ ...projectOn('p-home', 'local'), path: '/disk/projects/home' })
    expect(resolveWorkspaceRootPath('ws-home')).toBe('/disk/projects/home')
  })

  it('returns undefined for an unknown workspace', () => {
    expect(resolveWorkspaceRootPath('ws-unknown')).toBeUndefined()
  })
})
