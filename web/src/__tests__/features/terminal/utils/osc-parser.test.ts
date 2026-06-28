// web/src/__tests__/features/terminal/utils/osc-parser.test.ts
import { describe, it, expect } from 'vitest'
import { parseOSC7 } from '@/features/terminal/utils/osc-parser'

const osc7 = (path: string) => `\x1b]7;file://host${path}\x07`

describe('parseOSC7', () => {
  it('returns the only directory', () => {
    expect(parseOSC7(osc7('/a/b'))).toBe('/a/b')
  })

  it('returns the LAST directory when a burst contains several (replay case)', () => {
    const burst = `${osc7('/old/dir')}some output${osc7('/new/dir')}`
    expect(parseOSC7(burst)).toBe('/new/dir')
  })

  it('decodes percent-encoding', () => {
    expect(parseOSC7(osc7('/a%20b'))).toBe('/a b')
  })

  it('returns null when no OSC 7 present', () => {
    expect(parseOSC7('plain text')).toBeNull()
  })
})
