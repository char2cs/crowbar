import { describe, expect, it } from 'vitest'
import { openThreadCountsByPath } from '@/features/git/lib/review-thread-counts'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'

const thread = (id: string, filePath: string, isResolved = false): ReviewThread => ({
  id,
  filePath,
  lineNumber: 1,
  startLine: 1,
  endLine: 1,
  side: 'new',
  messages: [],
  isResolved,
})

describe('openThreadCountsByPath', () => {
  it('counts unresolved threads per file and ignores un-anchored ones', () => {
    const byPath = openThreadCountsByPath([
      thread('1', 'a.ts'),
      thread('2', 'a.ts'),
      thread('3', 'a.ts', true),
      thread('4', 'b.ts'),
      thread('5', ''),
    ])
    expect(byPath.get('a.ts')).toBe(2)
    expect(byPath.get('b.ts')).toBe(1)
    expect(byPath.has('')).toBe(false)
  })
})
