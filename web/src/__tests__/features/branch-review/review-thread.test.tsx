import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReviewThreadView } from '@/features/branch-review/components/review-thread'
import type { ReviewThread } from '@/features/branch-review/types/review-types'

const thread: ReviewThread = {
  id: 't1', filePath: 'src/index.ts', lineNumber: 10, side: 'right', isResolved: false,
  messages: [
    { id: 'm1', author: 'Claude', isAgent: true, body: 'Consider caching here.', createdAt: '2026-06-01' },
    { id: 'm2', author: null, isAgent: false, body: 'Good point!', createdAt: '2026-06-01' },
  ],
}

describe('ReviewThreadView', () => {
  it('renders agent messages with author name and badge', () => {
    render(<ReviewThreadView thread={thread} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('Claude')).toBeTruthy()
    expect(screen.getByText('agent')).toBeTruthy()
  })

  it('renders user messages as "You"', () => {
    render(<ReviewThreadView thread={thread} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('You')).toBeTruthy()
  })

  it('calls onReply with trimmed text on Enter', async () => {
    const onReply = vi.fn()
    render(<ReviewThreadView thread={thread} onReply={onReply} onResolve={() => {}} />)
    await userEvent.type(screen.getByPlaceholderText(/reply/i), 'My reply')
    await userEvent.keyboard('{Enter}')
    expect(onReply).toHaveBeenCalledWith('My reply')
  })

  it('clears the input after submit', async () => {
    render(<ReviewThreadView thread={thread} onReply={() => {}} onResolve={() => {}} />)
    const input = screen.getByPlaceholderText(/reply/i) as HTMLInputElement
    await userEvent.type(input, 'hello')
    await userEvent.keyboard('{Enter}')
    expect(input.value).toBe('')
  })

  it('calls onResolve when resolve button clicked', async () => {
    const onResolve = vi.fn()
    render(<ReviewThreadView thread={thread} onReply={() => {}} onResolve={onResolve} />)
    await userEvent.click(screen.getByRole('button', { name: /resolve/i }))
    expect(onResolve).toHaveBeenCalledOnce()
  })
})
