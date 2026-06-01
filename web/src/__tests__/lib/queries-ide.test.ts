// web/src/__tests__/lib/queries-ide.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn(),
  fetchWorkspace: vi.fn(),
  fetchFlows: vi.fn(),
  fetchConversation: vi.fn(),
}))

import * as api from '@/lib/api'
import { fileTreeQueryOptions, gitStatusQueryOptions, gitHistoryQueryOptions, gitBranchesQueryOptions } from '@/lib/queries'
import { getMockFileTree } from '@/lib/mock/files'
import { getMockGitStatus, getMockCommitHistory, getMockBranches } from '@/lib/mock/git-data'

beforeEach(() => {
  vi.resetAllMocks()
})

describe('IDE query options', () => {
  it('fileTreeQueryOptions has correct queryKey', () => {
    const opts = fileTreeQueryOptions('/workspace')
    expect(opts.queryKey).toEqual(['file-tree', '/workspace'])
  })

  it('fileTreeQueryOptions queryFn returns non-empty array', async () => {
    const mockTree = getMockFileTree('/workspace')
    vi.mocked(api.apiFetch).mockResolvedValueOnce(mockTree)
    const opts = fileTreeQueryOptions('/workspace')
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await (opts.queryFn as any)!()
    expect(Array.isArray(result)).toBe(true)
    expect((result as unknown[]).length).toBeGreaterThan(0)
  })

  it('gitStatusQueryOptions has correct queryKey', () => {
    const opts = gitStatusQueryOptions('/repo')
    expect(opts.queryKey).toEqual(['git-status', '/repo'])
  })

  it('gitStatusQueryOptions queryFn returns status object', async () => {
    vi.mocked(api.apiFetch).mockResolvedValueOnce(getMockGitStatus('/repo'))
    const opts = gitStatusQueryOptions('/repo')
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await (opts.queryFn as any)!()
    expect(result).toHaveProperty('files')
    expect(result).toHaveProperty('branch')
  })

  it('gitHistoryQueryOptions queryFn returns commits array', async () => {
    vi.mocked(api.apiFetch).mockResolvedValueOnce(getMockCommitHistory('/repo'))
    const opts = gitHistoryQueryOptions('/repo')
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await (opts.queryFn as any)!()
    expect(Array.isArray(result)).toBe(true)
    expect((result as unknown[]).length).toBeGreaterThanOrEqual(3)
  })

  it('gitBranchesQueryOptions queryFn returns branches array', async () => {
    vi.mocked(api.apiFetch).mockResolvedValueOnce(getMockBranches('/repo'))
    const opts = gitBranchesQueryOptions('/repo')
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await (opts.queryFn as any)!()
    expect(Array.isArray(result)).toBe(true)
    expect((result as unknown[]).some((b: unknown) => (b as { isCurrent: boolean }).isCurrent)).toBe(true)
  })
})
