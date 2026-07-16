import { beforeEach, describe, expect, it } from 'vitest'
import { resetWorkspaceScopedStores } from '@/features/workspace/lib/reset-workspace-scoped-stores'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useGitStore } from '@/features/git/stores/git-store'
import type { AppFile } from '@/features/file-system/types/app'
import type { GitStatus } from '@/features/git/types/git-types'

const fileA: AppFile = { name: 'scenario-01.ts', path: 'scenario-01.ts', isDir: false }

function seedWorkspaceA() {
  useFileSystemStore.setState({
    rootFolderPath: 'ws-A',
    files: [fileA],
    fileTree: [fileA],
    isFileTreeLoading: false,
  })
  const status: GitStatus = {
    branch: 'A',
    ahead: 0,
    behind: 0,
    files: [{ path: 'scenario-01.ts', status: 'added', staged: false }],
  }
  useGitStore.getState().actions.loadFreshGitData({
    gitStatus: status,
    commits: [{ hash: 'abc', message: 'a', author: 'a', date: '' }],
    branches: ['A'],
    stashes: [],
    repoPath: 'ws-A',
  })
}

beforeEach(() => {
  useFileSystemStore.setState({
    rootFolderPath: null,
    files: [],
    fileTree: [],
    isFileTreeLoading: false,
  })
  useGitStore.getState().actions.reset()
})

describe('resetWorkspaceScopedStores', () => {
  it('never leaves workspace A tree/git after switching to a workspace with no data', () => {
    seedWorkspaceA()
    // Sanity: A's data is loaded.
    expect(useFileSystemStore.getState().files).toHaveLength(1)
    expect(useGitStore.getState().commits).toHaveLength(1)

    // Activate B (which has no seeded data yet).
    resetWorkspaceScopedStores('ws-B')

    const fs = useFileSystemStore.getState()
    expect(fs.rootFolderPath).toBe('ws-B')
    expect(fs.files).toEqual([])
    expect(fs.fileTree).toEqual([])
    expect(fs.isFileTreeLoading).toBe(true)

    const git = useGitStore.getState()
    expect(git.commits).toEqual([])
    expect(git.branches).toEqual([])
    expect(git.workspaceGitStatus).toBeNull()
    expect(git.currentWorkspaceRepoPath).toBeNull()
    // The History tab renders from the gitData Loadable (idle → "Loading…"), so
    // a surviving success() would keep painting A's commits there.
    expect(git.gitData.status).toBe('idle')
  })

  it('clears stale tree/git on a home mount (home has no git surface / empty tree)', () => {
    seedWorkspaceA()

    // Home activates — it never refetches git, so a stale git panel would
    // otherwise keep showing A's status/commits.
    resetWorkspaceScopedStores('home-ws')

    expect(useFileSystemStore.getState().rootFolderPath).toBe('home-ws')
    expect(useFileSystemStore.getState().files).toEqual([])
    expect(useGitStore.getState().workspaceGitStatus).toBeNull()
    expect(useGitStore.getState().commits).toEqual([])
    // Home never refetches git, so a stale gitData success() would show A's
    // History indefinitely — it must drop back to idle.
    expect(useGitStore.getState().gitData.status).toBe('idle')
  })

  it('is a no-op when the stores already hold the activating workspace (warm return)', () => {
    seedWorkspaceA()
    const filesBefore = useFileSystemStore.getState().files
    const commitsBefore = useGitStore.getState().commits

    resetWorkspaceScopedStores('ws-A')

    // Same references preserved — no wipe of a warm workspace's own data.
    expect(useFileSystemStore.getState().files).toBe(filesBefore)
    expect(useGitStore.getState().commits).toBe(commitsBefore)
    expect(useFileSystemStore.getState().rootFolderPath).toBe('ws-A')
    // The Loadable keeps its loaded data too (no spurious "Loading…" flash).
    expect(useGitStore.getState().gitData.status).toBe('success')
  })
})
