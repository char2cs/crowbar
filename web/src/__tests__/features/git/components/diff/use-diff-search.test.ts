import { renderHook, act, waitFor } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { useDiffSearch } from '@/features/git/components/diff/use-diff-search'
import type { GitDiff } from '@/features/git/types/git-types'

// A minimal context-line diff: serializeGitDiffSourceForEditor reproduces the
// lines verbatim (no headers → no spacers), so model lines == array indices + 1.
function diff(path: string, lines: string[]): GitDiff {
  return {
    file_path: path,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: lines.map((content, i) => ({
      line_type: 'context' as const,
      content,
      old_line_number: i + 1,
      new_line_number: i + 1,
    })),
  } as GitDiff
}

const keyForIndex = (i: number) => `key-${i}`

describe('useDiffSearch', () => {
  it('computes matches across files and navigates with wraparound', async () => {
    const files = [diff('a.ts', ['foo here', 'and foo again']), diff('b.ts', ['foo three'])]
    const { result } = renderHook(() => useDiffSearch({ files, keyForIndex, enabled: true }))

    act(() => result.current.setQuery('foo'))
    await waitFor(() => expect(result.current.total).toBe(3))

    expect(result.current.currentIndex).toBe(0)
    expect(result.current.matchesByFile.get('key-0')).toHaveLength(2)
    expect(result.current.matchesByFile.get('key-1')).toHaveLength(1)

    act(() => result.current.next())
    expect(result.current.currentIndex).toBe(1)
    act(() => result.current.next())
    expect(result.current.currentIndex).toBe(2)
    act(() => result.current.next()) // wrap forward
    expect(result.current.currentIndex).toBe(0)
    act(() => result.current.prev()) // wrap back
    expect(result.current.currentIndex).toBe(2)
  })

  it('computes nothing while disabled', async () => {
    const files = [diff('a.ts', ['foo'])]
    const { result } = renderHook(() => useDiffSearch({ files, keyForIndex, enabled: false }))

    act(() => result.current.setQuery('foo'))
    await new Promise((resolve) => setTimeout(resolve, 200))
    expect(result.current.total).toBe(0)
    expect(result.current.current).toBeNull()
  })
})
