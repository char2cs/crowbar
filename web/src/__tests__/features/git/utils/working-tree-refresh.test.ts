import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/workspace-scope-url', () => ({
  workspaceBase: (wsId: string) => `/v0/ws/${wsId}`,
}))

import { getFileDiff } from '@/features/git/api/git-diff-api'
import { gitDiffCache } from '@/features/git/utils/git-diff-cache'
import {
  planWorkingTreeRefresh,
  refreshWorkingTreeMultiDiff,
  type WorkingTreeStatusEntry,
} from '@/features/git/utils/working-tree-multi-diff'
import type { GitDiff, GitFile, GitStatus } from '@/features/git/types/git-types'

const entry = (
  path: string,
  status: string,
  extra: Partial<WorkingTreeStatusEntry> = {},
): WorkingTreeStatusEntry => ({ path, status, ...extra })

describe('planWorkingTreeRefresh', () => {
  it('keeps a file whose path + status are unchanged', () => {
    const plan = planWorkingTreeRefresh([entry('a.ts', 'modified')], [entry('a.ts', 'modified')])
    expect(plan.keep).toEqual(['a.ts'])
    expect(plan.invalidate).toEqual([])
  })

  it('invalidates a newly appearing file', () => {
    const plan = planWorkingTreeRefresh(
      [entry('a.ts', 'modified')],
      [entry('a.ts', 'modified'), entry('b.ts', 'added')],
    )
    expect(plan.invalidate).toEqual(['b.ts'])
    expect(plan.keep).toEqual(['a.ts'])
  })

  it('invalidates a file whose status letter changed (added → modified)', () => {
    const plan = planWorkingTreeRefresh([entry('a.ts', 'added')], [entry('a.ts', 'modified')])
    expect(plan.invalidate).toEqual(['a.ts'])
    expect(plan.keep).toEqual([])
  })

  it('invalidates a file whose oid changed even when the status letter did not', () => {
    const plan = planWorkingTreeRefresh(
      [entry('a.ts', 'modified', { oid: '1111' })],
      [entry('a.ts', 'modified', { oid: '2222' })],
    )
    expect(plan.invalidate).toEqual(['a.ts'])
  })

  it('invalidates a file that became deleted', () => {
    const plan = planWorkingTreeRefresh([entry('a.ts', 'modified')], [entry('a.ts', 'deleted')])
    expect(plan.invalidate).toEqual(['a.ts'])
  })

  it('invalidates a file whose staged flag flipped', () => {
    const plan = planWorkingTreeRefresh(
      [entry('a.ts', 'modified', { staged: false })],
      [entry('a.ts', 'modified', { staged: true })],
    )
    expect(plan.invalidate).toEqual(['a.ts'])
  })

  it('treats a rename as delete + add — both paths invalidate', () => {
    const plan = planWorkingTreeRefresh(
      [entry('old.ts', 'modified')],
      [entry('old.ts', 'deleted'), entry('new.ts', 'added')],
    )
    expect(plan.invalidate.sort()).toEqual(['new.ts', 'old.ts'])
    expect(plan.keep).toEqual([])
  })

  it('does not list a reverted file (present in prev, absent in next) at all', () => {
    const plan = planWorkingTreeRefresh(
      [entry('a.ts', 'modified'), entry('b.ts', 'modified')],
      [entry('a.ts', 'modified')],
    )
    expect(plan.invalidate).toEqual([])
    expect(plan.keep).toEqual(['a.ts'])
  })
})

const status = (files: GitFile[]): GitStatus => ({
  branch: 'main',
  ahead: 0,
  behind: 0,
  files,
})

const file = (path: string, status: GitFile['status'] = 'modified'): GitFile => ({
  path,
  status,
  staged: false,
})

describe('refreshWorkingTreeMultiDiff', () => {
  it('invalidates only the paths the planner marks changed', async () => {
    const invalidate = vi.fn()
    const loadDiff = vi.fn(
      async (_repo: string, filePath: string): Promise<GitDiff> => ({
        file_path: filePath,
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        lines: [{ line_type: 'added', content: '+x', new_line_number: 1 }],
      }),
    )

    await refreshWorkingTreeMultiDiff({
      repoPath: '/repo',
      previousStatus: status([file('a.ts'), file('b.ts'), file('c.ts')]),
      nextStatus: status([file('a.ts'), file('b.ts'), file('c.ts'), file('d.ts', 'added')]),
      invalidate,
      loadDiff,
    })

    expect(invalidate).toHaveBeenCalledTimes(1)
    expect(invalidate).toHaveBeenCalledWith('/repo', 'd.ts')
  })

  // Acceptance (mechanism level): an N-file working tree with exactly one
  // changed status entry hits the network exactly once — the unchanged files
  // are served from the warm gitDiffCache.
  it('makes exactly one network fetch when one of N files changed', async () => {
    gitDiffCache.clear()
    apiFetch.mockImplementation(async (url: string) => {
      const path = decodeURIComponent(url.match(/path=([^&]+)/)?.[1] ?? '')
      return [
        {
          file_path: path,
          is_new: false,
          is_deleted: false,
          is_renamed: false,
          lines: [{ line_type: 'added', content: '+x', new_line_number: 1 }],
        },
      ] satisfies GitDiff[]
    })

    const warm = status([file('a.ts'), file('b.ts'), file('c.ts')])
    // Warm the cache for a, b, c (previousStatus null → all fetched once).
    await refreshWorkingTreeMultiDiff({
      repoPath: 'ws1',
      previousStatus: null,
      nextStatus: warm,
      loadDiff: getFileDiff,
    })
    expect(apiFetch).toHaveBeenCalledTimes(3)
    apiFetch.mockClear()

    // A single new file appears; a, b, c are unchanged.
    await refreshWorkingTreeMultiDiff({
      repoPath: 'ws1',
      previousStatus: warm,
      nextStatus: status([file('a.ts'), file('b.ts'), file('c.ts'), file('d.ts', 'added')]),
      loadDiff: getFileDiff,
    })

    expect(apiFetch).toHaveBeenCalledTimes(1)
    expect(apiFetch.mock.calls[0]?.[0]).toContain('path=d.ts')
  })

  beforeEach(() => {
    vi.clearAllMocks()
    gitDiffCache.clear()
  })
})
