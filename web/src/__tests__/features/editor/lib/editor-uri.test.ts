import { describe, expect, it } from 'vitest'
import { fileUri, uriToFsPath } from '@/features/editor/lib/editor-uri'

describe('editor-uri', () => {
  it('builds a stable crowbar uri from a file path (keyed by file, not buffer)', () => {
    const a = fileUri('/repo/src/index.ts')
    const b = fileUri('/repo/src/index.ts')
    expect(a).toBe(b)
    expect(a).toMatch(/^crowbar:\/\/editor\//)
  })

  it('round-trips back to the fs path', () => {
    expect(uriToFsPath(fileUri('/repo/a b/c.ts'))).toBe('/repo/a b/c.ts')
  })

  it('distinguishes different files', () => {
    expect(fileUri('/a.ts')).not.toBe(fileUri('/b.ts'))
  })
})
