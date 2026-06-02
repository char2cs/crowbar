import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/components/ui/toast', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

import { toast } from '@/components/ui/toast'
import { executeMerge } from '@/features/branch-review/components/merge-button'

beforeEach(() => { vi.clearAllMocks() })

describe('executeMerge', () => {
  it('shows success toast when onMerge resolves', async () => {
    await executeMerge(async () => {}, { title: 't', description: '' })
    expect(toast.success).toHaveBeenCalledWith('Merged successfully')
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('shows error toast when onMerge rejects', async () => {
    await executeMerge(async () => { throw new Error('boom') }, { title: 't', description: '' })
    expect(toast.error).toHaveBeenCalledWith('Merge failed — check logs')
    expect(toast.success).not.toHaveBeenCalled()
  })

  it('awaits a synchronous onMerge and still reports success', async () => {
    const onMerge = vi.fn()
    await executeMerge(onMerge, { title: 't', description: '' })
    expect(onMerge).toHaveBeenCalledOnce()
    expect(toast.success).toHaveBeenCalledWith('Merged successfully')
  })
})
