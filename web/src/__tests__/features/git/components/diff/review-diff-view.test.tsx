import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

// ── Module mocks ───────────────────────────────────────────────────────────────

// Mock buildDiffTokens to resolve null (graceful degradation, no tree-sitter needed)
vi.mock('@/features/git/lib/render-tree-sitter-token', async (importOriginal) => {
  const original = await importOriginal<typeof import('@/features/git/lib/render-tree-sitter-token')>()
  return {
    ...original,
    buildDiffTokens: vi.fn().mockResolvedValue(null),
  }
})

// Mock ImageDiffViewer so image-diff tests don't need blobs
vi.mock('@/features/git/components/diff/git-diff-image', () => ({
  default: ({ fileName }: { fileName: string }) => (
    <div data-testid="image-diff-viewer">{fileName}</div>
  ),
}))

// Flatten the virtualizer so every item renders immediately (no scroll container needed)
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count, estimateSize }: { count: number; estimateSize: (i: number) => number }) => ({
    getTotalSize: () =>
      Array.from({ length: count }, (_, i) => estimateSize(i)).reduce((a, b) => a + b, 0),
    getVirtualItems: () =>
      Array.from({ length: count }, (_, i) => ({
        index: i,
        start: 0,
        key: i,
        size: estimateSize(i),
        lane: 0,
        end: estimateSize(i),
      })),
    measureElement: () => undefined,
    scrollToIndex: () => undefined,
  }),
}))

// Mock useWorkspaceStoreContext so ReviewDiffView can render outside a provider in tests
vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStoreContext: (selector: (s: { branchReview: { activeFileKey: null; activeFileNonce: number } }) => unknown) =>
    selector({ branchReview: { activeFileKey: null, activeFileNonce: 0 } }),
}))

import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { GitDiff } from '@/features/git/types/git-types'
import { ReviewDiffView } from '@/features/git/components/diff/review-diff-view'

// ── Test fixtures ──────────────────────────────────────────────────────────────

function makeGitDiff(filePath: string, uncommitted = false): GitDiff {
  return {
    file_path: filePath,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    is_binary: false,
    is_image: false,
    uncommitted,
    lines: [
      { line_type: 'header', content: '@@ -1,2 +1,2 @@' },
      { line_type: 'removed', content: 'old line in ' + filePath, old_line_number: 1 },
      { line_type: 'added', content: 'new line in ' + filePath, new_line_number: 1 },
      { line_type: 'context', content: 'context line', old_line_number: 2, new_line_number: 2 },
    ],
  }
}

const twoFileDiff: MultiFileDiff = {
  commitHash: 'abc1234',
  files: [makeGitDiff('src/foo.ts', false), makeGitDiff('src/bar.ts', true)],
  totalFiles: 2,
  totalAdditions: 2,
  totalDeletions: 2,
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe('ReviewDiffView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders both file headers with their paths', () => {
    render(<ReviewDiffView multiDiff={twoFileDiff} />)

    expect(screen.getByText('src/foo.ts')).toBeDefined()
    expect(screen.getByText('src/bar.ts')).toBeDefined()
  })

  it('shows the uncommitted pill ONLY on the uncommitted file', () => {
    render(<ReviewDiffView multiDiff={twoFileDiff} />)

    // There should be exactly one "uncommitted" badge
    const badges = screen.getAllByText('uncommitted')
    expect(badges).toHaveLength(1)
  })

  it('renders each file diff body with changed line text (react-diff-view output)', async () => {
    const { container } = render(<ReviewDiffView multiDiff={twoFileDiff} />)

    // react-diff-view renders change content into <td> cells inside the diff table.
    // The diff bodies expand by default (shouldAutoCollapse = false for 2 lines).
    // Wait for the async tokenization promise (resolves null) to settle.
    await vi.waitFor(() => {
      const text = container.textContent ?? ''
      expect(text).toContain('new line in src/foo.ts')
      expect(text).toContain('new line in src/bar.ts')
    })
  })

  it('gracefully renders plain text when buildDiffTokens resolves null', async () => {
    const { container } = render(<ReviewDiffView multiDiff={twoFileDiff} />)

    await vi.waitFor(() => {
      // Both files should still show content even with no syntax tokens
      const text = container.textContent ?? ''
      expect(text).toContain('old line in src/foo.ts')
      expect(text).toContain('old line in src/bar.ts')
    })
  })

  it('renders footer with file count', () => {
    render(<ReviewDiffView multiDiff={twoFileDiff} />)
    expect(screen.getByText(/2 files changed/)).toBeDefined()
  })
})
