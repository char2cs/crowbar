import { describe, it, expect } from 'vitest'
import {
  registerBlock,
  findBlock,
  getBlock,
  allBlocks,
} from '@/features/markdown-chat/lib/block-registry'

const noopView = () => null

describe('block-registry', () => {
  it('registers and resolves a block by match + type', () => {
    registerBlock({
      type: 'foo',
      storage: 'inline',
      match: (info) => info.type === 'foo',
      View: noopView,
    })
    expect(getBlock('foo')?.type).toBe('foo')
    expect(findBlock({ type: 'foo', params: {}, meta: '' })?.type).toBe('foo')
    expect(findBlock({ type: 'nope', params: {}, meta: '' })).toBeUndefined()
  })

  it('replaces an entry of the same type instead of duplicating', () => {
    const countBefore = allBlocks().filter((b) => b.type === 'dup').length
    registerBlock({ type: 'dup', storage: 'inline', match: () => false, View: noopView })
    registerBlock({ type: 'dup', storage: 'referenced', match: () => false, View: noopView })
    expect(allBlocks().filter((b) => b.type === 'dup').length).toBe(countBefore + 1)
    expect(getBlock('dup')?.storage).toBe('referenced')
  })
})
