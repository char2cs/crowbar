import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ReviewConversationView } from '@/features/git/components/review-conversation-view'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

const mocks = vi.hoisted(() => ({
  openThread: vi.fn(),
  getIdentity: vi.fn(),
  getReviewPatch: vi.fn(),
}))
vi.mock('@/features/git/api/review-api', () => ({ openThread: mocks.openThread }))
vi.mock('@/features/git/api/identity-api', () => ({ getIdentity: mocks.getIdentity }))
vi.mock('@/features/git/api/review-window-api', () => ({ getReviewPatch: mocks.getReviewPatch }))

const PATCH = [
  'diff --git a/src/a.ts b/src/a.ts',
  '--- a/src/a.ts',
  '+++ b/src/a.ts',
  '@@ -1,6 +1,7 @@',
  ' one',
  ' two',
  ' three',
  '-four',
  '+FOUR',
  '+four-b',
  ' five',
  ' six',
  '',
].join('\n')

function thread(overrides: Partial<ReviewThread>): ReviewThread {
  return {
    id: 't',
    filePath: 'src/a.ts',
    lineNumber: 4,
    startLine: 4,
    endLine: 4,
    side: 'new',
    isResolved: false,
    messages: [
      {
        id: 'm',
        author: 'alice',
        isAgent: false,
        body: 'looks off',
        createdAt: '2026-01-01T00:00:00Z',
      },
    ],
    ...overrides,
  }
}

function renderView(threads: ReviewThread[], description = '') {
  const store = createWorkspaceStore('ws-1')
  store.getState().setBranchReviewDescription(description)
  for (const t of threads) store.getState().upsertReviewThread(t)
  const onJumpToLine = vi.fn()
  render(
    <WorkspaceStoreContext.Provider value={store}>
      <ReviewConversationView wsId="ws-1" onJumpToLine={onJumpToLine} />
    </WorkspaceStoreContext.Provider>,
  )
  return { onJumpToLine }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.getIdentity.mockResolvedValue({ login: 'me', displayName: 'Me' })
  mocks.openThread.mockResolvedValue(undefined)
  mocks.getReviewPatch.mockResolvedValue({ patch: PATCH, truncated: false })
})

describe('ReviewConversationView', () => {
  it('renders with no threads and no description (a branch with zero changed files)', () => {
    renderView([])
    expect(screen.queryByRole('region', { name: 'Description' })).toBeNull()
    expect(screen.queryByText('No description.')).toBeNull()
    expect(screen.queryByRole('region', { name: 'Merge' })).toBeNull()
    expect(screen.getByRole('textbox', { name: 'Leave a comment' })).toBeDefined()
  })

  it('shows anchored threads with file:line and a working Jump to line; un-anchored ones have none', async () => {
    const user = userEvent.setup()
    const anchored = thread({ id: 'a', filePath: 'src/a.ts', lineNumber: 4 })
    const general = thread({
      id: 'g',
      filePath: '',
      lineNumber: 0,
      startLine: 0,
      endLine: 0,
      messages: [
        {
          id: 'm2',
          author: 'bob',
          isAgent: false,
          body: 'overall note',
          createdAt: '2026-01-02T00:00:00Z',
        },
      ],
    })
    const { onJumpToLine } = renderView([anchored, general])

    expect(screen.getByText('looks off')).toBeDefined()
    expect(screen.getByText('overall note')).toBeDefined()
    expect(screen.getByText('src/a.ts:4')).toBeDefined()
    expect(screen.getByText('General comment')).toBeDefined()
    const jumps = screen.getAllByRole('button', { name: /Jump to line/ })
    expect(jumps).toHaveLength(1)

    await user.click(jumps[0])
    expect(onJumpToLine).toHaveBeenCalledWith(anchored)
  })

  it('folds a resolved thread to its header and opens it on demand', async () => {
    const user = userEvent.setup()
    renderView([thread({ id: 'r', isResolved: true })])
    expect(screen.getByText('Resolved')).toBeDefined()
    expect(screen.queryByText('looks off')).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Expand thread' }))
    expect(screen.getByText('looks off')).toBeDefined()
  })

  it('gives an open thread a Resolve button and a reply box', () => {
    renderView([thread({ id: 'o' })])
    expect(screen.getByRole('button', { name: 'Resolve' })).toBeDefined()
    expect(screen.getByPlaceholderText('Reply…')).toBeDefined()
  })

  it('reads the anchored file once and shows its code around the line, not for general comments', async () => {
    renderView([
      thread({ id: 'a', lineNumber: 5, startLine: 5 }),
      thread({ id: 'b', lineNumber: 5, startLine: 5 }),
      thread({ id: 'g', filePath: '', lineNumber: 0, startLine: 0, endLine: 0 }),
    ])
    await waitFor(() => expect(screen.getAllByTestId('review-thread-snippet')).toHaveLength(2))
    expect(mocks.getReviewPatch).toHaveBeenCalledTimes(1)
    expect(mocks.getReviewPatch).toHaveBeenCalledWith({ wsId: 'ws-1' }, 'src/a.ts')
  })

  it('orders description, threads, then the composer', () => {
    renderView([thread({ id: 'a' })], 'Adds the thing')
    const regions = [
      screen.getByRole('region', { name: 'Description' }),
      screen.getByRole('region', { name: 'Threads' }),
      screen.getByRole('textbox', { name: 'Leave a comment' }),
    ]
    for (let i = 1; i < regions.length; i++) {
      expect(
        regions[i - 1].compareDocumentPosition(regions[i]) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy()
    }
    expect(within(regions[0]).getByText('Adds the thing')).toBeDefined()
  })

  it('posts the general comment as an un-anchored thread and clears the box', async () => {
    const user = userEvent.setup()
    renderView([])
    const box = screen.getByRole('textbox', { name: 'Leave a comment' })
    expect(box.getAttribute('placeholder')).toBe(
      'Agents working on this branch pick this up on their own.',
    )
    const send = screen.getByRole('button', { name: 'Comment' })
    expect((send as HTMLButtonElement).disabled).toBe(true)

    await user.type(box, 'Please add tests')
    await user.click(send)

    expect(mocks.openThread).toHaveBeenCalledWith('ws-1', {
      filePath: '',
      line: 0,
      startLine: 0,
      endLine: 0,
      side: 'new',
      author: 'me',
      isAgent: false,
      body: 'Please add tests',
    })
    expect((box as HTMLTextAreaElement).value).toBe('')
  })

  it('keeps the text and says so when posting fails', async () => {
    const user = userEvent.setup()
    mocks.openThread.mockRejectedValue(new Error('502'))
    renderView([])
    const box = screen.getByRole('textbox', { name: 'Leave a comment' })

    await user.type(box, 'draft')
    await user.click(screen.getByRole('button', { name: 'Comment' }))

    expect(await screen.findByText('Failed to post comment')).toBeDefined()
    expect((box as HTMLTextAreaElement).value).toBe('draft')
  })
})
