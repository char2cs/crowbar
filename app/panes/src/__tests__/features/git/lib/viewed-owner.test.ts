import { describe, expect, it } from 'vitest'
import { IdbViewedOwner } from '@/features/git/lib/idb-viewed-owner'
import { installViewedOwner, viewedOwner } from '@/features/git/lib/viewed-owner'

describe('viewedOwner', () => {
  it('is the owner the entry point installed', () => {
    const owner = new IdbViewedOwner()
    installViewedOwner(owner)
    expect(viewedOwner()).toBe(owner)
  })
})
