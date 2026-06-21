import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

const mockOpenThread = vi.fn()
vi.mock('@/features/git/api/review-api', () => ({
  openThread: (...args: unknown[]) => mockOpenThread(...args),
  replyToThread: vi.fn().mockResolvedValue({ id: 'thread1', messages: [], isResolved: false }),
  setThreadResolved: vi.fn().mockResolvedValue({ id: 'thread1', messages: [], isResolved: true }),
}))

vi.mock('@/features/git/hooks/use-current-identity', () => ({
  useCurrentIdentity: () => ({
    login: 'testuser',
    displayName: 'Test User',
    avatarUrl: '',
  }),
}))

// Track the store state for mutation testing
let storeState = {
  workspaceId: 'ws-test',
  branchReview: {
    activeFileKey: null as string | null,
    activeFileNonce: 0,
    threads: [] as import('@/features/workspace/stores/slices/branch-review-slice').ReviewThread[],
  },
}

const mockUpsertReviewThread = vi.fn()
const mockRemoveReviewThread = vi.fn()
const mockGetState = vi.fn(() => ({
  upsertReviewThread: mockUpsertReviewThread,
  removeReviewThread: mockRemoveReviewThread,
}))

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStoreContext: (selector: (s: typeof storeState) => unknown) => selector(storeState),
  useWorkspaceStore: () => ({ getState: mockGetState }),
}))

import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { GitDiff } from '@/features/git/types/git-types'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
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

function makeThread(overrides: Partial<ReviewThread> = {}): ReviewThread {
  return {
    id: 'thread1',
    filePath: 'src/foo.ts',
    lineNumber: 1,
    startLine: 1,
    endLine: 1,
    side: 'new',
    messages: [
      {
        id: 'msg1',
        author: 'alice',
        isAgent: false,
        body: 'Hello there',
        createdAt: '2024-01-01T00:00:00Z',
      },
    ],
    isResolved: false,
    ...overrides,
  }
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe('ReviewDiffView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    storeState = {
      workspaceId: 'ws-test',
      branchReview: {
        activeFileKey: null,
        activeFileNonce: 0,
        threads: [],
      },
    }
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

  it('does not throw and shows empty state when files is null', () => {
    expect(() => {
      render(<ReviewDiffView multiDiff={{ files: null } as unknown as MultiFileDiff} />)
    }).not.toThrow()
    expect(screen.getByText(/No changes to show/)).toBeDefined()
  })

  it('does not throw and shows empty state when multiDiff has no files property', () => {
    expect(() => {
      render(<ReviewDiffView multiDiff={{} as unknown as MultiFileDiff} />)
    }).not.toThrow()
    expect(screen.getByText(/No changes to show/)).toBeDefined()
  })

  it('does not throw and shows empty state when files is an empty array', () => {
    const emptyDiff: MultiFileDiff = {
      commitHash: '',
      files: [],
      totalFiles: 0,
      totalAdditions: 0,
      totalDeletions: 0,
    }
    expect(() => {
      render(<ReviewDiffView multiDiff={emptyDiff} />)
    }).not.toThrow()
    expect(screen.getByText(/No changes to show/)).toBeDefined()
  })

  it('thread in store renders as inline widget at its line', async () => {
    // Thread on src/foo.ts at line 1 (insert — new_line_number: 1)
    storeState.branchReview.threads = [makeThread()]

    const { container } = render(<ReviewDiffView multiDiff={twoFileDiff} />)

    await vi.waitFor(() => {
      // ReviewThreadItem renders the author initials in the message row
      const text = container.textContent ?? ''
      expect(text).toContain('Hello there')
    })
  })

  it('gutter + click opens CommentComposer', async () => {
    const user = userEvent.setup()
    render(<ReviewDiffView multiDiff={twoFileDiff} />)

    // Wait for diff to render
    await vi.waitFor(() => {
      expect(screen.queryByText('new line in src/foo.ts')).not.toBeNull()
    })

    // Find a gutter button (the "+" comment button)
    const gutterBtns = document.querySelectorAll('.diff-comment-gutter-btn')
    expect(gutterBtns.length).toBeGreaterThan(0)

    await user.click(gutterBtns[0] as HTMLElement)

    // CommentComposer should appear (it renders a "Comment" submit button)
    await vi.waitFor(() => {
      expect(screen.queryByText('Comment')).not.toBeNull()
    })
  })

  it('submit calls openThread with correct anchor and author', async () => {
    const user = userEvent.setup()

    mockOpenThread.mockResolvedValue(makeThread())
    mockUpsertReviewThread.mockImplementation(() => {})

    render(<ReviewDiffView multiDiff={twoFileDiff} />)

    // Wait for diff to render
    await vi.waitFor(() => {
      expect(screen.queryByText('new line in src/foo.ts')).not.toBeNull()
    })

    // Click gutter button on insert line (new side)
    const gutterBtns = document.querySelectorAll('.diff-comment-gutter-btn')
    expect(gutterBtns.length).toBeGreaterThan(0)
    await user.click(gutterBtns[0] as HTMLElement)

    // Wait for CommentComposer
    await vi.waitFor(() => {
      expect(screen.queryByText('Comment')).not.toBeNull()
    })

    // Type a comment and submit
    const editor = document.querySelector('.cm-content') as HTMLElement
    if (editor) {
      await user.click(editor)
      await user.type(editor, 'My inline comment')
    }

    // Click the Comment submit button
    const submitBtn = screen.getByText('Comment')
    await user.click(submitBtn)

    // openThread should have been called
    await vi.waitFor(() => {
      expect(mockOpenThread).toHaveBeenCalledWith(
        'ws-test',
        expect.objectContaining({
          filePath: 'src/foo.ts',
          author: 'testuser',
          isAgent: false,
        }),
      )
    })
  })
})
