import { describe, expect, it } from 'vitest'
import { editorTarget } from '@/entries/editor-target'

describe('editorTarget', () => {
  it('reads a workspace file, a host file and an untitled file', () => {
    expect(editorTarget({ path: 'src/a.ts' })).toEqual({
      kind: 'workspace',
      path: 'src/a.ts',
      fresh: false,
      line: null,
    })
    expect(editorTarget({ path: '/home/a.ts', absolute: true, fresh: true, line: 3 })).toEqual({
      kind: 'host',
      path: '/home/a.ts',
      fresh: true,
      line: 3,
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
