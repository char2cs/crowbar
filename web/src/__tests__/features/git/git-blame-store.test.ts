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

describe('git-blame-store memory management', () => {
  beforeEach(async () => {
    const { useGitBlameStore } = await import('@/features/git/stores/git-blame-store')
    useGitBlameStore.setState({ blame: new Map(), fileToRepo: new Map() })
  })

  it('clearBlameForFile removes only the targeted file entry', async () => {
    const { useGitBlameStore } = await import('@/features/git/stores/git-blame-store')
    await useGitBlameStore.getState().loadBlameForFile('/repo', 'a.ts')
    await useGitBlameStore.getState().loadBlameForFile('/repo', 'b.ts')

    useGitBlameStore.getState().clearBlameForFile('a.ts')

    expect(useGitBlameStore.getState().blame.has('a.ts')).toBe(false)
    expect(useGitBlameStore.getState().fileToRepo.has('a.ts')).toBe(false)
    // b.ts must remain untouched
    expect(useGitBlameStore.getState().blame.has('b.ts')).toBe(true)
    expect(useGitBlameStore.getState().fileToRepo.has('b.ts')).toBe(true)
  })

  it('clearBlameForFile is a no-op for a file that was never loaded', async () => {
    const { useGitBlameStore } = await import('@/features/git/stores/git-blame-store')
    await useGitBlameStore.getState().loadBlameForFile('/repo', 'a.ts')

    expect(() => useGitBlameStore.getState().clearBlameForFile('never-opened.ts')).not.toThrow()
    expect(useGitBlameStore.getState().blame.has('a.ts')).toBe(true)
  })

  it('clearAllBlame empties both maps', async () => {
    const { useGitBlameStore } = await import('@/features/git/stores/git-blame-store')
    await useGitBlameStore.getState().loadBlameForFile('/repo', 'a.ts')
    await useGitBlameStore.getState().loadBlameForFile('/repo', 'b.ts')
    await useGitBlameStore.getState().loadBlameForFile('/repo', 'c.ts')

    useGitBlameStore.getState().clearAllBlame()

    expect(useGitBlameStore.getState().blame.size).toBe(0)
    expect(useGitBlameStore.getState().fileToRepo.size).toBe(0)
  })
})
