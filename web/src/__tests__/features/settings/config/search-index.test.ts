import { describe, it, expect } from 'vitest'
import { settingsSearchIndex } from '@/features/settings/config/search-index'

describe('settings search index', () => {
  it('has unique search-record ids', () => {
    const ids = settingsSearchIndex.map((r) => r.id)
    expect(new Set(ids).size).toBe(ids.length)
  })
})
