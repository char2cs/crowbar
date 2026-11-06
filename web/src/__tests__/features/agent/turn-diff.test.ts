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

  it('reads a tool edit’s bare unified diff as a change to its file, not a rename', () => {
    const edit =
      '--- a/app.py\n+++ b/app.py\n@@ -1 +1 @@\n-old\n+new\n--- /dev/null\n+++ b/n.md\n@@ -0,0 +1 @@\n+one\n'
    const files = parseTurnDiff(edit, 't').map((i) => i.type === 'diff' && i.fileDiff)
    expect(files).toMatchObject([{ name: 'app.py', type: 'change' }, { name: 'n.md' }])
    expect(files.map((f) => f && f.prevName)).toEqual([undefined, undefined])
  })

  it('declines oversized input before parsing', () => {
    expect(parseTurnDiff('x'.repeat(MAX_TURN_DIFF_PREVIEW_CHARS + 1), 'turn-1')).toEqual([])
  })
})
