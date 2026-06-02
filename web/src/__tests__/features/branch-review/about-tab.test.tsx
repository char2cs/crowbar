import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { success, failed, idle } from '@/lib/loadable'
import { useBranchReviewChatsStore } from '@/features/branch-review/stores/branch-review-data-store'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'
import { AboutTab } from '@/features/branch-review/components/about-tab'

const data = (chats: { id: string; title: string; age: string; isActive: boolean }[]) =>
  chats as unknown as BranchReviewChat[]

beforeEach(() => { useBranchReviewChatsStore.setState({ data: idle() }) })

const noop = () => {}

describe('AboutTab conversations', () => {
  it('renders chats on success', () => {
    useBranchReviewChatsStore.setState({ data: success(data([{ id: 'c1', title: 'Arch review', age: '2h', isActive: true }])) })
    render(<AboutTab wsId="ws3" description="" onDescriptionChange={noop} onOpenConversation={noop} onNewConversation={noop} />)
    expect(screen.getByText('Arch review')).toBeInTheDocument()
  })

  it('shows inline error when chats fail with no cache', () => {
    useBranchReviewChatsStore.setState({ data: failed(new Error('500'), idle()) })
    render(<AboutTab wsId="ws3" description="" onDescriptionChange={noop} onOpenConversation={noop} onNewConversation={noop} />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
  })
})

describe('AboutTab — new conversation', () => {
  it('renders a new conversation button', () => {
    const onNewConversation = vi.fn()
    render(<AboutTab wsId="ws3" description="" onDescriptionChange={noop} onOpenConversation={noop} onNewConversation={onNewConversation} />)
    expect(screen.getByRole('button', { name: 'New conversation' })).toBeInTheDocument()
  })

  it('calls onNewConversation when the + button is clicked', async () => {
    const onNewConversation = vi.fn()
    render(<AboutTab wsId="ws3" description="" onDescriptionChange={noop} onOpenConversation={noop} onNewConversation={onNewConversation} />)
    await userEvent.click(screen.getByRole('button', { name: 'New conversation' }))
    expect(onNewConversation).toHaveBeenCalledOnce()
  })
})
