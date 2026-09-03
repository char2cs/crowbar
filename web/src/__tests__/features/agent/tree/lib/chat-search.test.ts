import { describe, expect, it } from 'vitest'
import { searchChats, splitHighlight } from '@/features/agent/tree/lib/chat-search'
import type { SearchNode } from '@/features/agent/tree/lib/chat-search'

const n = (id: string, title: string, children: SearchNode[] = []): SearchNode => ({
  id,
  title,
  children,
})

const roots: SearchNode[] = [
  n('f1', 'Release 1.4', [
    n('c1', 'Ship the sidebar redesign', [n('c2', 'Drag ghost drifts off the cursor')]),
    n('c3', 'Changelog for 1.4'),
  ]),
  n('c4', 'Drag rules for the chats tree'),
]

describe('searchChats', () => {
  it('an empty query matches nothing and keeps nothing', () => {
    const { match, keep, ctx } = searchChats(roots, '')
    expect(match.size).toBe(0)
    expect(keep.size).toBe(0)
    expect(ctx.size).toBe(0)
  })

  it('a whitespace-only query is treated as empty', () => {
    expect(searchChats(roots, '   ').keep.size).toBe(0)
  })

  it('matches case-insensitively on a substring', () => {
    expect([...searchChats(roots, 'DRAG').match].sort()).toEqual(['c2', 'c4'])
  })

  it('keeps ancestors of a match, and marks them as context', () => {
    const { keep, ctx } = searchChats(roots, 'drag')
    expect(keep.has('f1')).toBe(true)
    expect(keep.has('c1')).toBe(true)
    expect(ctx.has('f1')).toBe(true)
    expect(ctx.has('c1')).toBe(true)
  })

  it('never marks a match itself as context, even when it is also an ancestor', () => {
    // c1 matches AND is c2's parent: it must render as a hit, not as scaffolding.
    const { match, ctx } = searchChats(roots, 'sidebar')
    expect(match.has('c1')).toBe(true)
    expect(ctx.has('c1')).toBe(false)
  })

  it('keeps the whole subtree under a matched row', () => {
    expect(searchChats(roots, 'sidebar').keep.has('c2')).toBe(true)
  })

  it('drops branches with no match anywhere in them', () => {
    expect(searchChats(roots, 'drag').keep.has('c3')).toBe(false)
  })

  it('keeps nothing when the query matches nothing', () => {
    const { match, keep } = searchChats(roots, 'zzz')
    expect(match.size).toBe(0)
    expect(keep.size).toBe(0)
  })

  it('handles an empty tree', () => {
    expect(searchChats([], 'drag').match.size).toBe(0)
  })
})

describe('splitHighlight', () => {
  it('splits around the first case-insensitive hit', () => {
    expect(splitHighlight('Drag rules', 'rul')).toEqual({
      before: 'Drag ',
      hit: 'rul',
      after: 'es',
    })
  })

  it('preserves the original casing of the hit', () => {
    expect(splitHighlight('Drag rules', 'DRAG').hit).toBe('Drag')
  })

  it('returns the whole title as before when there is no hit', () => {
    expect(splitHighlight('Drag rules', 'zzz')).toEqual({
      before: 'Drag rules',
      hit: '',
      after: '',
    })
  })

  it('returns the whole title as before for an empty query', () => {
    expect(splitHighlight('Drag rules', '')).toEqual({ before: 'Drag rules', hit: '', after: '' })
  })

  it('returns the whole title as before for a whitespace-only query', () => {
    expect(splitHighlight('Drag rules', '  ')).toEqual({ before: 'Drag rules', hit: '', after: '' })
  })
})
