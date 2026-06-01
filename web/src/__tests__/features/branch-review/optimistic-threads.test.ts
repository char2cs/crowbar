import { describe, it, expect, vi } from 'vitest'
import { addThreadOptimistic } from '@/features/workspace/stores/slices/branch-review-slice'

describe('addThreadOptimistic', () => {
  it('applies the change and does not roll back when commit succeeds', async () => {
    const apply = vi.fn()
    const rollback = vi.fn()
    const committed = await addThreadOptimistic({ apply, rollback, commit: async () => {} })
    expect(apply).toHaveBeenCalledOnce()
    expect(rollback).not.toHaveBeenCalled()
    expect(committed).toBe(true)
  })

  it('rolls back and returns false when commit fails', async () => {
    const apply = vi.fn()
    const rollback = vi.fn()
    const committed = await addThreadOptimistic({ apply, rollback, commit: async () => { throw new Error('x') } })
    expect(apply).toHaveBeenCalledOnce()
    expect(rollback).toHaveBeenCalledOnce()
    expect(committed).toBe(false)
  })

  it('applies before commit (ordering)', async () => {
    const calls: string[] = []
    await addThreadOptimistic({
      apply: () => calls.push('apply'),
      rollback: () => calls.push('rollback'),
      commit: async () => { calls.push('commit') },
    })
    expect(calls).toEqual(['apply', 'commit'])
  })
})
