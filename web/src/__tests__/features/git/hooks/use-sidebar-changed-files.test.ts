import { renderHook, act, cleanup } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import type { GitDiff, GitFile, GitStatus } from '@/features/git/types/git-types'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { useSidebarChangedFiles } from '@/features/git/hooks/use-sidebar-changed-files'

function sf(path: string, status: GitFile['status'], staged = false): GitFile {
  return { path, status, staged }
}

function makeStatus(files: GitFile[]): GitStatus {
  return { branch: 'feature', ahead: 0, behind: 0, files }
}

function summaryDiff(path: string, extra: Partial<GitDiff> = {}): GitDiff {
  return {
    file_path: path,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: [],
    ...extra,
  }
}

function setState(wsId: string, state: { gitStatus?: GitStatus; reviewFiles?: GitDiff[] }) {
  act(() => getOrCreateWorkspaceStore(wsId).setState(state))
}

describe('useSidebarChangedFiles', () => {
  afterEach(() => {
    // Unmount hooks BEFORE tearing down the stores they subscribe to.
    cleanup()
    getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  })

  it('returns empty when wsId is null', () => {
    const { result } = renderHook(() => useSidebarChangedFiles(null))
    expect(result.current.files).toEqual([])
    expect(result.current.uncommittedCount).toBe(0)
  })

  it('paints the status projection until the workspace’s summary has loaded, then upgrades to it', () => {
    getOrCreateWorkspaceStore('ws-closed')
    setState('ws-closed', {
      gitStatus: makeStatus([sf('src/a.ts', 'modified'), sf('src/b.ts', 'added')]),
    })

    const { result } = renderHook(() => useSidebarChangedFiles('ws-closed'))

    expect(result.current.files.map((f) => f.file_path)).toEqual(['src/a.ts', 'src/b.ts'])
    expect(result.current.files[1].is_new).toBe(true)
    expect(result.current.files[0].additions).toBeUndefined()

    setState('ws-closed', {
      reviewFiles: [
        summaryDiff('src/a.ts', { additions: 3, deletions: 1, uncommitted: true }),
        summaryDiff('src/b.ts', { additions: 5, uncommitted: true }),
      ],
    })
    expect(result.current.files[0].additions).toBe(3)
    expect(result.current.files[0].deletions).toBe(1)
    expect(result.current.uncommittedCount).toBe(2)
  })

  it('shows committed-only files that the status projection lacks', () => {
    getOrCreateWorkspaceStore('ws-committed')
    setState('ws-committed', {
      gitStatus: makeStatus([sf('src/wip.ts', 'modified')]),
      reviewFiles: [
        summaryDiff('src/committed.ts', { additions: 10 }),
        summaryDiff('src/wip.ts', { additions: 2, uncommitted: true }),
      ],
    })

    const { result } = renderHook(() => useSidebarChangedFiles('ws-committed'))

    expect(result.current.files.map((f) => f.file_path)).toEqual(['src/committed.ts', 'src/wip.ts'])
    expect(result.current.uncommittedCount).toBe(1)
  })

  // Switching the focused workspace selects another slice: no empty frame and
  // nothing of the previous workspace's list.
  it('follows the workspace it is given without leaking the previous one’s files', () => {
    getOrCreateWorkspaceStore('ws-a')
    getOrCreateWorkspaceStore('ws-b')
    setState('ws-a', { reviewFiles: [summaryDiff('a.ts')] })
    setState('ws-b', { reviewFiles: [summaryDiff('b.ts')] })

    const { result, rerender } = renderHook(({ ws }) => useSidebarChangedFiles(ws), {
      initialProps: { ws: 'ws-a' },
    })
    expect(result.current.files.map((f) => f.file_path)).toEqual(['a.ts'])

    rerender({ ws: 'ws-b' })
    expect(result.current.files.map((f) => f.file_path)).toEqual(['b.ts'])
  })
})
