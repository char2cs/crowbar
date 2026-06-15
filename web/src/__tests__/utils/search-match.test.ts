import { describe, it, expect } from 'vitest'
import { fuzzyMatch } from '@/utils/search-match'

describe('fuzzyMatch', () => {
  it('matches multiple tokens in any order across separators', () => {
    const text = 'crowbar / feature/wave2-engines'
    expect(fuzzyMatch('crowbar wave2', text)).toBe(true)
    expect(fuzzyMatch('wave2 crowbar', text)).toBe(true)
  })

  it('matches each token as a subsequence (e.g. abbreviations)', () => {
    expect(fuzzyMatch('qvr shell', 'quiver.desktop / feature/quiver-shell')).toBe(true)
    expect(fuzzyMatch('init scaffold', 'asyn\\x / mateo-urrutia/init-scaffolding')).toBe(true)
  })

  it('is case- and accent-insensitive', () => {
    expect(fuzzyMatch('DEVELOP', 'crowbar / develop')).toBe(true)
  })

  it('returns true for an empty/whitespace query', () => {
    expect(fuzzyMatch('', 'crowbar / develop')).toBe(true)
    expect(fuzzyMatch('   ', 'crowbar / develop')).toBe(true)
  })

  it('requires every token to match', () => {
    expect(fuzzyMatch('crowbar zzz', 'crowbar / develop')).toBe(false)
  })

  it('returns false when nothing matches', () => {
    expect(fuzzyMatch('zzzzz', 'crowbar / develop')).toBe(false)
  })
})
