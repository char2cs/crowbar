import { describe, it, expect } from 'vitest'
import { branchDiffQueryOptions, branchThreadsQueryOptions, branchDescriptionQueryOptions, branchChatsQueryOptions } from '@/features/branch-review/queries'

describe('branch-review query options', () => {
  it('branchDiffQueryOptions has correct queryKey', () => {
    const opts = branchDiffQueryOptions('ws3')
    expect(opts.queryKey).toEqual(['branch-diff', 'ws3'])
  })

  it('branchThreadsQueryOptions has correct queryKey', () => {
    const opts = branchThreadsQueryOptions('ws3')
    expect(opts.queryKey).toEqual(['branch-threads', 'ws3'])
  })

  it('branchDescriptionQueryOptions has correct queryKey', () => {
    const opts = branchDescriptionQueryOptions('ws3')
    expect(opts.queryKey).toEqual(['branch-description', 'ws3'])
  })

  it('branchChatsQueryOptions has correct queryKey', () => {
    const opts = branchChatsQueryOptions('ws3')
    expect(opts.queryKey).toEqual(['branch-chats', 'ws3'])
  })
})
