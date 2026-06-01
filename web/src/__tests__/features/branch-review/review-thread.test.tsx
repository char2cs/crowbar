import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReviewThreadView } from '@/features/branch-review/components/review-thread'
import type { ReviewThread } from '@/features/branch-review/types/review-types'

const sentThread: ReviewThread = {
  id: 't1', filePath: 'src/index.ts', lineNumber: 10, side: 'right', isResolved: false,
  messages: [
    { id: 'm1', author: 'Claude', isAgent: true, body: 'Consider caching here.', createdAt: '2026-06-01' },
    { id: 'm2', author: null, isAgent: false, body: 'Good point!', createdAt: '2026-06-01' },
  ],
}

const draftThread: ReviewThread = {
  id: 't2', filePath: 'src/index.ts', lineNumber: 7, side: 'right', isResolved: false,
  messages: [],
}

const noop = () => {}

describe('ReviewThreadView', () => {
  it('renders agent messages with author name and badge', () => {
    render(<ReviewThreadView thread={sentThread} onReply={noop} onResolve={noop} onDelete={noop} />)
    expect(screen.getByText('Claude')).toBeTruthy()
    expect(screen.getByText('agent')).toBeTruthy()
  })

  it('renders user messages as "You"', () => {
    render(<ReviewThreadView thread={sentThread} onReply={noop} onResolve={noop} onDelete={noop} />)
    expect(screen.getByText('You')).toBeTruthy()
  })

  it('shows Resolve on a sent thread and calls onResolve', async () => {
    const onResolve = vi.fn()
    render(<ReviewThreadView thread={sentThread} onReply={noop} onResolve={onResolve} onDelete={noop} />)
    await userEvent.click(screen.getByRole('button', { name: /resolve/i }))
    expect(onResolve).toHaveBeenCalledOnce()
  })

  it('renders a draft as a composer (no Resolve) and Cancel discards it', async () => {
    const onDelete = vi.fn()
    render(<ReviewThreadView thread={draftThread} onReply={noop} onResolve={noop} onDelete={onDelete} />)
    // Composer header for the draft
    expect(screen.getByText(/add a comment on line R7/i)).toBeTruthy()
    // No Resolve until the thread is actually sent
    expect(screen.queryByRole('button', { name: /resolve/i })).toBeNull()
    // Cancel discards the unsent draft
    await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onDelete).toHaveBeenCalledOnce()
  })

  it('exposes a Comment submit button in the draft composer', () => {
    render(<ReviewThreadView thread={draftThread} onReply={noop} onResolve={noop} onDelete={noop} />)
    expect(screen.getByRole('button', { name: /^comment$/i })).toBeTruthy()
  })
})
