import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ReviewThreadItem } from '@/features/git/components/review-thread-item'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import type { IdentityDTO } from '@/features/git/api/identity-api'

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))

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

// ── Three-dots comment menu (edit / copy / delete) ───────────────────────────────

const IDENTITY: IdentityDTO = {
  login: 'alice',
  displayName: 'Alice',
  avatarUrl: 'https://github.com/alice.png',
}

const manageProps = {
  wsId: 'ws-test',
  onReply: vi.fn().mockResolvedValue(undefined),
  onResolve: vi.fn().mockResolvedValue(undefined),
  onReopen: vi.fn().mockResolvedValue(undefined),
  onEditMessage: vi.fn().mockResolvedValue(undefined),
  onDeleteMessage: vi.fn().mockResolvedValue(undefined),
  onDeleteThread: vi.fn().mockResolvedValue(undefined),
}

// A thread authored by `alice` with one reply by `bob`.
function makeThreadWithReply(): ReviewThread {
  return makeThread({
    messages: [
      { id: 'root-1', author: 'alice', isAgent: false, body: 'root body', createdAt: '2024-01-01T00:00:00Z' },
      { id: 'reply-1', author: 'bob', isAgent: false, body: 'reply body', createdAt: '2024-01-02T00:00:00Z' },
    ],
  })
}

describe('ReviewThreadItem — comment menu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a menu trigger per message when manage handlers are provided', () => {
    render(<ReviewThreadItem thread={makeThreadWithReply()} {...manageProps} currentIdentity={IDENTITY} />)
    expect(screen.getAllByLabelText('Comment actions')).toHaveLength(2)
  })

  it('does not render a menu trigger without manage handlers', () => {
    render(<ReviewThreadItem thread={makeThreadWithReply()} {...defaultProps} currentIdentity={IDENTITY} />)
    expect(screen.queryByLabelText('Comment actions')).toBeNull()
  })

  it('shows Edit on your own non-agent comment', async () => {
    const user = userEvent.setup()
    render(<ReviewThreadItem thread={makeThreadWithReply()} {...manageProps} currentIdentity={IDENTITY} />)

    // First message is alice's (own) → Edit present.
    await user.click(screen.getAllByLabelText('Comment actions')[0])
    expect(await screen.findByRole('menuitem', { name: /Edit/ })).toBeDefined()
  })

  it('hides Edit on a comment you did not author, but keeps Copy + Delete', async () => {
    const user = userEvent.setup()
    render(<ReviewThreadItem thread={makeThreadWithReply()} {...manageProps} currentIdentity={IDENTITY} />)

    // Second message is bob's → no Edit, but Copy + Delete still present.
    await user.click(screen.getAllByLabelText('Comment actions')[1])
    expect(await screen.findByRole('menuitem', { name: /Copy as Markdown/ })).toBeDefined()
    expect(screen.getByRole('menuitem', { name: /Delete/ })).toBeDefined()
    expect(screen.queryByRole('menuitem', { name: /Edit/ })).toBeNull()
  })

  it('deleting the root comment confirms then calls onDeleteThread', async () => {
    const user = userEvent.setup()
    render(<ReviewThreadItem thread={makeThreadWithReply()} {...manageProps} currentIdentity={IDENTITY} />)

    await user.click(screen.getAllByLabelText('Comment actions')[0])
    await user.click(await screen.findByRole('menuitem', { name: /Delete/ }))

    // Confirmation dialog for the whole thread.
    expect(await screen.findByText('Delete this thread?')).toBeDefined()
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    expect(manageProps.onDeleteThread).toHaveBeenCalledWith('thread1')
    expect(manageProps.onDeleteMessage).not.toHaveBeenCalled()
  })

  it('deleting a reply confirms then calls onDeleteMessage with the reply id', async () => {
    const user = userEvent.setup()
    render(<ReviewThreadItem thread={makeThreadWithReply()} {...manageProps} currentIdentity={IDENTITY} />)

    await user.click(screen.getAllByLabelText('Comment actions')[1])
    await user.click(await screen.findByRole('menuitem', { name: /Delete/ }))

    expect(await screen.findByText('Delete this comment?')).toBeDefined()
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    expect(manageProps.onDeleteMessage).toHaveBeenCalledWith('thread1', 'reply-1')
    expect(manageProps.onDeleteThread).not.toHaveBeenCalled()
  })

  it('editing a comment opens an editor seeded with the body and saves via onEditMessage', async () => {
    const user = userEvent.setup()
    render(<ReviewThreadItem thread={makeThreadWithReply()} {...manageProps} currentIdentity={IDENTITY} />)

    await user.click(screen.getAllByLabelText('Comment actions')[0])
    await user.click(await screen.findByRole('menuitem', { name: /Edit/ }))

    // The editor seeds with the existing body and offers Save.
    const save = await screen.findByRole('button', { name: 'Save' })
    await user.click(save)

    expect(manageProps.onEditMessage).toHaveBeenCalledWith('thread1', 'root-1', 'root body')
  })

  it('Copy as Markdown writes the body to the clipboard', async () => {
    const user = userEvent.setup()
    // Override after setup() — userEvent installs its own clipboard stub on setup.
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
      writable: true,
    })
    render(<ReviewThreadItem thread={makeThreadWithReply()} {...manageProps} currentIdentity={IDENTITY} />)

    await user.click(screen.getAllByLabelText('Comment actions')[1])
    await user.click(await screen.findByRole('menuitem', { name: /Copy as Markdown/ }))

    expect(writeText).toHaveBeenCalledWith('reply body')
  })
})
