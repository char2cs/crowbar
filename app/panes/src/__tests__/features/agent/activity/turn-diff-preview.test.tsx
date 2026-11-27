import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

const seen = vi.hoisted(() => ({ options: [] as unknown[] }))
vi.mock('@/features/git/components/diff/diff-code-view', () => ({
  DiffCodeView: ({ options }: { options: unknown }) => {
    seen.options.push(options)
    return null
  },
}))

import { MAX_TURN_DIFF_PREVIEW_CHARS } from '@/features/agent/lib/turn-diff'
const { TurnDiffPreview } = await import('@/features/agent/activity/turn-diff-preview')

const DIFF = [
  'diff --git a/a.ts b/a.ts',
  '--- a/a.ts',
  '+++ b/a.ts',
  '@@ -1,2 +1,2 @@',
  ' one',
  '-two',
  '+TWO',
  '@@ -40,2 +40,2 @@',
  ' forty',
  '-fortyone',
  '+FORTYONE',
  '',
].join('\n')

describe('TurnDiffPreview', () => {
  it('separates hunks with a plain divider', () => {
    render(<TurnDiffPreview diff={DIFF} turnId="t1" />)
    expect(seen.options.at(-1)).toMatchObject({ hunkSeparators: 'simple' })
  })

  it('names the full diff and its size when it exceeds the live-render cap', () => {
    const diff = 'x'.repeat(MAX_TURN_DIFF_PREVIEW_CHARS + 1)
    render(<TurnDiffPreview diff={diff} turnId="t1" />)
    expect(screen.getByText(/full diff is 244\.1 KB.*too large to render live/i)).toBeTruthy()
  })
})
