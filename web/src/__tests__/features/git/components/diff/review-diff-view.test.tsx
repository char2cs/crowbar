import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

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

vi.mock('@/features/git/hooks/use-git-diff-highlight', () => ({
  useDiffHighlighting: () => new Map(),
}))

vi.mock('@/features/editor/stores/settings-store', () => ({
  useEditorSettingsStore: Object.assign(
    (selector: (s: { fontSize: number; fontFamily: string; tabSize: number; wordWrap: boolean }) => unknown) =>
      selector({ fontSize: 13, fontFamily: 'monospace', tabSize: 2, wordWrap: false }),
    {
      use: {
        fontSize: () => 13,
        fontFamily: () => 'monospace',
        tabSize: () => 2,
        wordWrap: () => false,
      },
    },
  ),
}))

vi.mock('@/features/window/stores/zoom-store', () => ({
  useZoomStore: Object.assign(
    (selector: (s: { editorZoomLevel: number }) => unknown) =>
      selector({ editorZoomLevel: 1 }),
    {
      use: {
        editorZoomLevel: () => 1,
      },
    },
  ),
}))

vi.mock('@/features/editor/hooks/use-selection-scope', () => ({
  useSelectionScope: () => undefined,
}))

vi.mock('@/features/git/components/diff/git-diff-image', () => ({
  default: ({ fileName }: { fileName: string }) => (
    <div data-testid="image-diff-viewer">{fileName}</div>
  ),
}))

let storeState = {
  workspaceId: 'ws-test',
  branchReview: {
    activeFileKey: null as string | null,
    activeFileNonce: 0,
    threads: [],
  },
}

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStoreContext: (selector: (s: typeof storeState) => unknown) => selector(storeState),
  useWorkspaceStore: () => ({ getState: vi.fn(() => ({})) }),
}))

import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { GitDiff } from '@/features/git/types/git-types'
import { ReviewDiffView } from '@/features/git/components/diff/review-diff-view'

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

    const badges = screen.getAllByText('uncommitted')
    expect(badges).toHaveLength(1)
  })

  it('renders each expanded file diff body with changed line text via native TextDiffViewer', () => {
    const { container } = render(<ReviewDiffView multiDiff={twoFileDiff} />)

    const text = container.textContent ?? ''
    expect(text).toContain('new line in src/foo.ts')
    expect(text).toContain('new line in src/bar.ts')
  })

  it('renders removed lines via native TextDiffViewer', () => {
    const { container } = render(<ReviewDiffView multiDiff={twoFileDiff} />)

    const text = container.textContent ?? ''
    expect(text).toContain('old line in src/foo.ts')
    expect(text).toContain('old line in src/bar.ts')
  })

  it('renders footer with file count', () => {
    render(<ReviewDiffView multiDiff={twoFileDiff} />)
    expect(screen.getAllByText(/2 files/).length).toBeGreaterThan(0)
  })

  it('shows header bar with Unified/Split toggles and Whitespace toggle', () => {
    render(<ReviewDiffView multiDiff={twoFileDiff} />)
    expect(screen.getByText('Unified')).toBeDefined()
    expect(screen.getByText('Split')).toBeDefined()
    expect(screen.getByText('Whitespace')).toBeDefined()
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
})
