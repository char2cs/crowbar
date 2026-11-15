import { describe, expect, it } from 'vitest'
import { editorTarget } from '@/entries/editor-target'

describe('editorTarget', () => {
  it('reads a workspace file, a host file and an untitled file', () => {
    expect(editorTarget({ path: 'src/a.ts' })).toEqual({ kind: 'workspace', path: 'src/a.ts' })
    expect(editorTarget({ path: '/home/a.ts', absolute: true })).toEqual({
      kind: 'host',
      path: '/home/a.ts',
    })
    expect(editorTarget({ untitled: true })).toEqual({ kind: 'untitled' })
  })

  it.each([null, undefined, 'a.ts', {}, { path: '' }, { path: 3 }])(
    'names nothing for %j',
    (saved) => {
      expect(editorTarget(saved)).toBeNull()
    },
  )
})
