import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resolveWorkspaceRootPath } from '@/lib/workspace/resolve-root-path'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'

const { getActiveWorkspaceId } = vi.hoisted(() => ({
  getActiveWorkspaceId: vi.fn<() => string | null>(() => null),
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId,
}))

const repo: Repo = {
  id: 'r1',
  name: 'athas',
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
  useSidebarStore.setState({ repos: [repo] })
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
