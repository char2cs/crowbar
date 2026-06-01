import { describe, it, expect, beforeEach } from 'vitest'
import { saveBranchReview, loadBranchReview } from '@/lib/persistence/branch-review'
import { resetDB } from '@/lib/persistence/idb'

beforeEach(() => resetDB())

describe('branch-review persistence', () => {
  it('returns null when nothing has been saved', async () => {
    const result = await loadBranchReview('ws-test')
    expect(result).toBeNull()
  })

  it('saves and loads branch review state', async () => {
    await saveBranchReview('ws-test', {
      description: '# My branch',
      mergeStrategy: 'squash',
      activeSubtab: 'diff',
      threads: [],
    })
    const loaded = await loadBranchReview('ws-test')
    expect(loaded?.description).toBe('# My branch')
    expect(loaded?.mergeStrategy).toBe('squash')
    expect(loaded?.activeSubtab).toBe('diff')
    expect(loaded?.threads).toEqual([])
  })

  it('overwrites previous state on save', async () => {
    await saveBranchReview('ws-test', { description: 'v1', mergeStrategy: 'merge', activeSubtab: 'about', threads: [] })
    await saveBranchReview('ws-test', { description: 'v2', mergeStrategy: 'rebase', activeSubtab: 'git', threads: [] })
    const loaded = await loadBranchReview('ws-test')
    expect(loaded?.description).toBe('v2')
    expect(loaded?.mergeStrategy).toBe('rebase')
  })

  it('persists threads across save/load', async () => {
    const threads = [{
      id: 't1', filePath: 'src/index.ts', lineNumber: 5,
      side: 'right' as const, isResolved: false,
      messages: [{ id: 'm1', author: 'Claude', isAgent: true, body: 'Nice!', createdAt: '2026-06-01' }],
    }]
    await saveBranchReview('ws-test', { description: '', mergeStrategy: 'merge', activeSubtab: 'about', threads })
    const loaded = await loadBranchReview('ws-test')
    expect(loaded?.threads).toHaveLength(1)
    expect(loaded?.threads[0].messages[0].author).toBe('Claude')
  })
})
