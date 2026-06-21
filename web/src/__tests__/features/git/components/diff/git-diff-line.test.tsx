/**
 * Tests for git-diff-line.tsx inline comment threading:
 * - gutter "+" button calls onAddComment with the correct anchor
 * - threads passed for a line render a ReviewThreadItem row
 */
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock the review API — openThread / replyToThread / setThreadResolved are called
// inside DiffLine's handlers, but we only need to verify the gutter interactions
// here. The API calls are tested in review-api.test.ts.
vi.mock('@/features/git/api/review-api', () => ({
  openThread: vi.fn().mockResolvedValue({ id: 'new-thread' }),
  replyToThread: vi.fn().mockResolvedValue({}),
  setThreadResolved: vi.fn().mockResolvedValue({}),
}))

// Mock CommentComposer so we can control its presence without CodeMirror.
vi.mock('@/features/panes/components/comment-composer', () => ({
  CommentComposer: ({
    onSubmit,
    onCancel,
    title,
  }: {
    onSubmit: (body: string) => void
    onCancel: () => void
    title?: string
  }) => (
    <div data-testid="comment-composer">
      {title && <span data-testid="composer-title">{title}</span>}
      <button onClick={() => onSubmit('test body')}>Submit</button>
      <button onClick={onCancel}>Cancel</button>
    </div>
  ),
}))

// Mock ReviewThreadItem — we just need to assert it's rendered, not exercise it.
vi.mock('@/features/git/components/review-thread-item', () => ({
  ReviewThreadItem: ({ thread }: { thread: { id: string; messages: { body: string }[] } }) => (
    <div data-testid={`thread-item-${thread.id}`}>{thread.messages[0]?.body}</div>
  ),
}))

import DiffLine from '@/features/git/components/diff/git-diff-line'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'

const BASE_PROPS = {
  viewMode: 'unified' as const,
  wordWrap: false,
  showWhitespace: false,
  fontSize: 13,
  lineHeight: 22,
  tabSize: 2,
}

function makeThread(overrides: Partial<ReviewThread> = {}): ReviewThread {
  return {
    id: 'thread-1',
    filePath: 'src/foo.ts',
    lineNumber: 5,
    startLine: 5,
    endLine: 5,
    side: 'new',
    messages: [{ id: 'msg1', author: 'alice', isAgent: false, body: 'A comment', createdAt: '' }],
    isResolved: false,
    ...overrides,
  }
}

describe('DiffLine — gutter "+" button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders "+" button for an added line and calls onAddComment with new side', () => {
    const onAddComment = vi.fn()
    const line = {
      line_type: 'added' as const,
      content: 'new line',
      new_line_number: 5,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/foo.ts"
        wsId="ws-1"
        threads={[]}
        onAddComment={onAddComment}
      />,
    )

    const btn = screen.getByTestId('add-comment-btn')
    expect(btn).toBeDefined()
    fireEvent.click(btn)

    expect(onAddComment).toHaveBeenCalledWith({
      filePath: 'src/foo.ts',
      side: 'new',
      line: 5,
    })
  })

  it('calls onAddComment with old side for a removed line', () => {
    const onAddComment = vi.fn()
    const line = {
      line_type: 'removed' as const,
      content: 'old line',
      old_line_number: 3,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/bar.ts"
        wsId="ws-1"
        threads={[]}
        onAddComment={onAddComment}
      />,
    )

    const btn = screen.getByTestId('add-comment-btn')
    fireEvent.click(btn)

    expect(onAddComment).toHaveBeenCalledWith({
      filePath: 'src/bar.ts',
      side: 'old',
      line: 3,
    })
  })

  it('calls onAddComment with new side for a context line', () => {
    const onAddComment = vi.fn()
    const line = {
      line_type: 'context' as const,
      content: 'ctx',
      old_line_number: 10,
      new_line_number: 10,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/ctx.ts"
        wsId="ws-1"
        threads={[]}
        onAddComment={onAddComment}
      />,
    )

    const btn = screen.getByTestId('add-comment-btn')
    fireEvent.click(btn)

    expect(onAddComment).toHaveBeenCalledWith({
      filePath: 'src/ctx.ts',
      side: 'new',
      line: 10,
    })
  })

  it('does NOT render "+" when onAddComment is not provided', () => {
    const line = {
      line_type: 'added' as const,
      content: 'new line',
      new_line_number: 5,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/foo.ts"
        wsId="ws-1"
        threads={[]}
      />,
    )

    expect(screen.queryByTestId('add-comment-btn')).toBeNull()
  })

  it('does NOT render "+" for header lines', () => {
    const onAddComment = vi.fn()
    const line = {
      line_type: 'header' as const,
      content: '@@ -1,3 +1,3 @@',
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/foo.ts"
        wsId="ws-1"
        threads={[]}
        onAddComment={onAddComment}
      />,
    )

    expect(screen.queryByTestId('add-comment-btn')).toBeNull()
  })

  it('opens an inline CommentComposer below the line after clicking "+"', () => {
    const onAddComment = vi.fn()
    const line = {
      line_type: 'added' as const,
      content: 'new line',
      new_line_number: 7,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/foo.ts"
        wsId="ws-1"
        threads={[]}
        onAddComment={onAddComment}
      />,
    )

    expect(screen.queryByTestId('comment-composer')).toBeNull()
    fireEvent.click(screen.getByTestId('add-comment-btn'))
    expect(screen.getByTestId('comment-composer')).toBeDefined()
  })

  it('closes the composer on Cancel', () => {
    const onAddComment = vi.fn()
    const line = {
      line_type: 'added' as const,
      content: 'new line',
      new_line_number: 7,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/foo.ts"
        wsId="ws-1"
        threads={[]}
        onAddComment={onAddComment}
      />,
    )

    fireEvent.click(screen.getByTestId('add-comment-btn'))
    expect(screen.getByTestId('comment-composer')).toBeDefined()

    fireEvent.click(screen.getByText('Cancel'))
    expect(screen.queryByTestId('comment-composer')).toBeNull()
  })
})

describe('DiffLine — thread rows', () => {
  it('renders a ReviewThreadItem for a thread anchored to this line', () => {
    const thread = makeThread({ lineNumber: 5, side: 'new' })
    const line = {
      line_type: 'added' as const,
      content: 'new line',
      new_line_number: 5,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/foo.ts"
        wsId="ws-1"
        threads={[thread]}
        onAddComment={vi.fn()}
      />,
    )

    expect(screen.getByTestId('thread-item-thread-1')).toBeDefined()
    expect(screen.getByText('A comment')).toBeDefined()
  })

  it('does NOT render a thread that belongs to a different line', () => {
    const thread = makeThread({ lineNumber: 99, side: 'new' })
    const line = {
      line_type: 'added' as const,
      content: 'new line',
      new_line_number: 5,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/foo.ts"
        wsId="ws-1"
        threads={[thread]}
        onAddComment={vi.fn()}
      />,
    )

    expect(screen.queryByTestId('thread-item-thread-1')).toBeNull()
  })

  it('does NOT render a thread that belongs to a different file', () => {
    const thread = makeThread({ lineNumber: 5, side: 'new', filePath: 'src/other.ts' })
    const line = {
      line_type: 'added' as const,
      content: 'new line',
      new_line_number: 5,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/foo.ts"
        wsId="ws-1"
        threads={[thread]}
        onAddComment={vi.fn()}
      />,
    )

    expect(screen.queryByTestId('thread-item-thread-1')).toBeNull()
  })

  it('does NOT render a thread when wsId is absent', () => {
    const thread = makeThread({ lineNumber: 5, side: 'new' })
    const line = {
      line_type: 'added' as const,
      content: 'new line',
      new_line_number: 5,
    }

    render(
      <DiffLine
        {...BASE_PROPS}
        line={line}
        filePath="src/foo.ts"
        threads={[thread]}
        onAddComment={vi.fn()}
      />,
    )

    // wsId is absent so thread rows are not rendered
    expect(screen.queryByTestId('thread-item-thread-1')).toBeNull()
  })
})
