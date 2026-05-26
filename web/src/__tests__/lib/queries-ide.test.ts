// web/src/__tests__/lib/queries-ide.test.ts
import { describe, it, expect } from 'vitest'
import { fileTreeQueryOptions, gitStatusQueryOptions, gitHistoryQueryOptions, gitBranchesQueryOptions } from '@/lib/queries'

describe('IDE query options', () => {
  it('fileTreeQueryOptions has correct queryKey', () => {
    const opts = fileTreeQueryOptions('/workspace')
    expect(opts.queryKey).toEqual(['file-tree', '/workspace'])
  })

  it('fileTreeQueryOptions queryFn returns non-empty array', async () => {
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
    const opts = gitStatusQueryOptions('/repo')
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await (opts.queryFn as any)!()
    expect(result).toHaveProperty('staged')
    expect(result).toHaveProperty('unstaged')
    expect(result).toHaveProperty('branch')
  })

  it('gitHistoryQueryOptions queryFn returns commits array', async () => {
    const opts = gitHistoryQueryOptions('/repo')
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await (opts.queryFn as any)!()
    expect(Array.isArray(result)).toBe(true)
    expect((result as unknown[]).length).toBeGreaterThanOrEqual(3)
  })

  it('gitBranchesQueryOptions queryFn returns branches array', async () => {
    const opts = gitBranchesQueryOptions('/repo')
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await (opts.queryFn as any)!()
    expect(Array.isArray(result)).toBe(true)
    expect((result as unknown[]).some((b: unknown) => (b as { isCurrent: boolean }).isCurrent)).toBe(true)
  })
})
