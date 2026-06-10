import { describe, expect, it } from 'vitest'
import { removeWorktree } from '@/features/git/api/git-worktrees-api'

// NOTE: git-worktrees-api uses a crowbar stub that throws "Not implemented".
// These tests verify the error handling wrappers return false gracefully.
describe('git worktrees api (stub behaviour)', () => {
  it('returns false when removeWorktree stub is not implemented', async () => {
    await expect(removeWorktree('/repo', '/repo/worktree')).resolves.toBe(false)
  })

  it('returns false with force=true when stub is not implemented', async () => {
    await expect(removeWorktree('/repo', '/repo/worktree', true)).resolves.toBe(false)
  })
})
