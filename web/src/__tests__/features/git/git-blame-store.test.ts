import { describe, it, expect, beforeEach, vi } from 'vitest'
import { dataOf } from '@/lib/loadable'

vi.mock('@/features/git/api/git-blame-api', () => ({
  getGitBlame: vi.fn(async () => ({ lines: [{ line_number: 1, total_lines: 1 }] })),
}))

describe('git-blame-store loadable', () => {
  beforeEach(async () => {
    const { useGitBlameStore } = await import('@/features/git/stores/git-blame-store')
    useGitBlameStore.setState({ blame: new Map(), fileToRepo: new Map() })
  })

  it('loads blame into a per-file Loadable', async () => {
    const { useGitBlameStore } = await import('@/features/git/stores/git-blame-store')
    await useGitBlameStore.getState().loadBlameForFile('/repo', 'a.ts')
    const entry = useGitBlameStore.getState().blame.get('a.ts')
    expect(entry?.status).toBe('success')
    expect(dataOf(entry)?.lines).toHaveLength(1)
  })

  it('getBlameForLine returns the line via dataOf', async () => {
    const { useGitBlameStore } = await import('@/features/git/stores/git-blame-store')
    await useGitBlameStore.getState().loadBlameForFile('/repo', 'a.ts')
    expect(useGitBlameStore.getState().getBlameForLine('a.ts', 1)).toEqual({
      line_number: 1,
      total_lines: 1,
    })
  })
})
