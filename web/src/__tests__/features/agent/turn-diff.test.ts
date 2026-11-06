import { describe, expect, it } from 'vitest'
import { MAX_TURN_DIFF_PREVIEW_CHARS, parseTurnDiff } from '@/features/agent/lib/turn-diff'

const patch = [
  'diff --git a/main.go b/main.go',
  '--- a/main.go',
  '+++ b/main.go',
  '@@ -1 +1 @@',
  '-old',
  '+new',
  '',
  'diff --git a/readme.md b/readme.md',
  '--- a/readme.md',
  '+++ b/readme.md',
  '@@ -1 +1 @@',
  '-before',
  '+after',
  '',
].join('\n')

describe('parseTurnDiff', () => {
  it('keeps each changed file as an ordered CodeView item', () => {
    const items = parseTurnDiff(patch, 'turn-1')

    expect(items).toHaveLength(2)
    expect(items.map((item) => item.type === 'diff' && item.fileDiff.name)).toEqual([
      'main.go',
      'readme.md',
    ])
    expect(items[0]?.id).not.toBe(items[1]?.id)
  })

  it('retains stable item IDs and changes the render version as snapshots update', () => {
    const first = parseTurnDiff(patch, 'turn-1')
    const second = parseTurnDiff(patch.replace('+new', '+newer'), 'turn-1')

    expect(second.map((item) => item.id)).toEqual(first.map((item) => item.id))
    expect(second[0]?.version).not.toBe(first[0]?.version)
  })

  // The daemon's per-edit diffs carry git's a/ b/ prefixes without a `diff --git` line.
  it('reads an edit with git prefixes but no git header as a change, not a rename', () => {
    const edit = ['--- a/src/calc.py', '+++ b/src/calc.py', '@@ -1 +1,2 @@', ' x', '+y', ''].join('\n')
    const [item] = parseTurnDiff(edit, 'turn-1')

    expect(item?.type === 'diff' && item.fileDiff).toMatchObject({
      name: 'src/calc.py',
      prevName: undefined,
      type: 'change',
    })
  })

  it('still reads a real rename as one', () => {
    const rename = ['--- a/old.py', '+++ b/new.py', '@@ -1 +1 @@', '-x', '+y', ''].join('\n')
    const [item] = parseTurnDiff(rename, 'turn-1')

    expect(item?.type === 'diff' && item.fileDiff).toMatchObject({
      name: 'new.py',
      prevName: 'old.py',
      type: 'rename-changed',
    })
  })

  it('declines oversized input before parsing', () => {
    expect(parseTurnDiff('x'.repeat(MAX_TURN_DIFF_PREVIEW_CHARS + 1), 'turn-1')).toEqual([])
  })
})
