import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ReviewThreadItem } from '@/features/git/components/review-thread-item'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'

// Mock @base-ui/react/avatar so AvatarImage unconditionally renders its <img>
// (jsdom never fires image load events, so base-ui's status stays 'idle'→null)
vi.mock('@base-ui/react/avatar', () => {
  const React = require('react')
  return {
    Avatar: {
      Root: ({ children, className }: { children: React.ReactNode; className?: string }) =>
        React.createElement('div', { className }, children),
      Image: ({ src, alt }: { src?: string; alt?: string }) =>
        src ? React.createElement('img', { src, alt }) : null,
      Fallback: ({ children }: { children: React.ReactNode }) =>
        React.createElement('span', {}, children),
    },
  }
})

// ── Fixtures ────────────────────────────────────────────────────────────────────

function makeThread(overrides: Partial<ReviewThread> = {}): ReviewThread {
  return {
    id: 'thread1',
    filePath: 'src/foo.ts',
    lineNumber: 5,
    startLine: 5,
    endLine: 5,
    side: 'new',
    messages: [
      {
        id: 'msg1',
        author: 'alice',
        isAgent: false,
        body: 'Hello **world**',
        createdAt: '2024-01-01T00:00:00Z',
      },
    ],
    isResolved: false,
    ...overrides,
  }
}

const noop = vi.fn().mockResolvedValue(undefined)

const defaultProps = {
  wsId: 'ws-test',
  onReply: noop,
  onResolve: noop,
  onReopen: noop,
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe('ReviewThreadItem', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders markdown body (bold text)', () => {
    const thread = makeThread()
    render(<ReviewThreadItem thread={thread} {...defaultProps} />)

    // "world" should be bolded — react-markdown renders <strong>world</strong>
    const bold = document.querySelector('strong')
    expect(bold).not.toBeNull()
    expect(bold?.textContent).toBe('world')
  })

  it('AGENT badge shown for agent messages', () => {
    const thread = makeThread({
      messages: [
        {
          id: 'msg1',
          author: 'bot',
          isAgent: true,
          body: 'I am an agent',
          createdAt: '2024-01-01T00:00:00Z',
        },
      ],
    })

    render(<ReviewThreadItem thread={thread} {...defaultProps} />)

    // The 'agent' badge should be visible
    expect(screen.getByText('agent')).toBeDefined()
  })

  it('Resolve calls onResolve with the thread id', async () => {
    const user = userEvent.setup()
    const onResolve = vi.fn().mockResolvedValue(undefined)
    const thread = makeThread()

    render(<ReviewThreadItem thread={thread} {...defaultProps} onResolve={onResolve} />)

    const resolveBtn = screen.getByText('Resolve')
    await user.click(resolveBtn)

    expect(onResolve).toHaveBeenCalledWith('thread1')
  })

  it('Reopen shown when resolved; clicking calls onReopen', async () => {
    const user = userEvent.setup()
    const onReopen = vi.fn().mockResolvedValue(undefined)
    const thread = makeThread({ isResolved: true })

    render(<ReviewThreadItem thread={thread} {...defaultProps} onReopen={onReopen} />)

    // Reopen button should appear for resolved threads
    const reopenBtn = screen.getByText('Reopen')
    expect(reopenBtn).toBeDefined()

    await user.click(reopenBtn)
    expect(onReopen).toHaveBeenCalledWith('thread1')
  })

  it('Resolve button NOT shown when thread is already resolved', () => {
    const thread = makeThread({ isResolved: true })
    render(<ReviewThreadItem thread={thread} {...defaultProps} />)

    expect(screen.queryByText('Resolve')).toBeNull()
  })

  it('outdated: shows Outdated badge and no thread content initially', () => {
    const thread = makeThread()
    render(<ReviewThreadItem thread={thread} {...defaultProps} isOutdated />)

    expect(screen.getByText('Outdated')).toBeDefined()
    // Thread message should NOT be visible before expansion
    expect(screen.queryByText('Hello')).toBeNull()
  })

  it('outdated: clicking Show expands thread content', async () => {
    const user = userEvent.setup()
    const thread = makeThread()

    render(<ReviewThreadItem thread={thread} {...defaultProps} isOutdated />)

    const showBtn = screen.getByText('Show')
    await user.click(showBtn)

    // After expanding, the message body should be visible
    await vi.waitFor(() => {
      expect(screen.queryByText('world')).not.toBeNull()
    })
  })

  it('non-outdated thread renders messages immediately', () => {
    const thread = makeThread()
    render(<ReviewThreadItem thread={thread} {...defaultProps} />)

    // The author name is shown
    expect(screen.getByText('alice')).toBeDefined()
  })

  it('human message with author renders GitHub profile photo img', () => {
    const thread = makeThread({
      messages: [
        {
          id: 'msg1',
          author: 'char2cs',
          isAgent: false,
          body: 'A human comment',
          createdAt: '2024-01-01T00:00:00Z',
        },
      ],
    })
    render(<ReviewThreadItem thread={thread} {...defaultProps} />)

    const imgs = document.querySelectorAll('img')
    const githubImg = Array.from(imgs).find((img) =>
      img.getAttribute('src')?.includes('github.com/char2cs.png'),
    )
    expect(githubImg).not.toBeUndefined()
  })

  it('agent message does NOT render a GitHub profile photo img', () => {
    const thread = makeThread({
      messages: [
        {
          id: 'msg1',
          author: 'char2cs',
          isAgent: true,
          body: 'An agent comment',
          createdAt: '2024-01-01T00:00:00Z',
        },
      ],
    })
    render(<ReviewThreadItem thread={thread} {...defaultProps} />)

    const imgs = document.querySelectorAll('img')
    const githubImg = Array.from(imgs).find((img) =>
      img.getAttribute('src')?.includes('github.com'),
    )
    expect(githubImg).toBeUndefined()
  })

  it('Reply box appears as CommentComposer with Reply submit button', async () => {
    const user = userEvent.setup()
    const thread = makeThread()

    render(<ReviewThreadItem thread={thread} {...defaultProps} />)

    const replyBtn = screen.getByPlaceholderText('Reply…')
    await user.click(replyBtn)

    // CommentComposer shows a "Reply" submit button
    await vi.waitFor(() => {
      expect(screen.queryByText('Reply')).not.toBeNull()
    })
  })
})
