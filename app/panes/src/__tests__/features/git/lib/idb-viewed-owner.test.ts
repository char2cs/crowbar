import { describe, expect, it, vi } from 'vitest'
import { IdbViewedOwner } from '@/features/git/lib/idb-viewed-owner'
import { loadBranchReviewViewed } from '@/lib/persistence/branch-review-viewed'

const a = { path: 'a.ts', signature: 'M||1|0' }
const b = { path: 'b.ts', signature: 'A||3|0' }

describe('IdbViewedOwner', () => {
  it('keeps the map in IndexedDB and tells followers of each batch, in order', async () => {
    const owner = new IdbViewedOwner()
    const heard = vi.fn()
    owner.follow('idb-1', heard)
    void owner.set('idb-1', { files: [a, b], viewed: true })
    await owner.set('idb-1', { files: [a], viewed: false })
    expect(await loadBranchReviewViewed('idb-1')).toEqual({ 'b.ts': b.signature })
    expect(heard.mock.calls.at(-1)).toEqual([{ 'b.ts': b.signature }])
  })

  it('gives a new follower the saved map, and an empty one for a workspace never marked', async () => {
    const owner = new IdbViewedOwner()
    await owner.set('idb-2', { files: [a], viewed: true })
    const saved = await new Promise((resolve) => owner.follow('idb-2', resolve))
    const fresh = await new Promise((resolve) => owner.follow('idb-3', resolve))
    expect([saved, fresh]).toEqual([{ 'a.ts': a.signature }, {}])
  })
})
