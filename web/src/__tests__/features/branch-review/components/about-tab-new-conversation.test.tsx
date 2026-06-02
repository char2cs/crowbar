import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { AboutTab } from '@/features/branch-review/components/about-tab'

vi.mock('@/features/branch-review/stores/branch-review-data-store', () => {
  const mockStoreState = {
    data: { status: 'success', data: { chats: [] }, fetchedAt: 0 },
    fetch: vi.fn(),
    startSync: vi.fn(),
    applyDelta: vi.fn(),
    optimisticWrite: vi.fn(),
  }

  const listeners = new Set<(state: any) => void>()

  const storeImpl = {
    ...mockStoreState,
    subscribe: (listener: (state: any) => void) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    getState: () => storeImpl,
    setState: (partial: any) => {
      Object.assign(storeImpl, partial)
      listeners.forEach(l => l(storeImpl))
    },
  }

  const mockUseBranchReviewDataStore = ((selector?: (state: any) => any) => {
    return selector ? selector(storeImpl) : storeImpl
  }) as any

  mockUseBranchReviewDataStore.getState = () => storeImpl

  return {
    useBranchReviewDataStore: mockUseBranchReviewDataStore,
  }
})

vi.mock('@uiw/react-codemirror', () => ({ default: () => null }))
vi.mock('@codemirror/lang-markdown', () => ({ markdown: () => ({}) }))

const baseProps = {
  wsId: 'ws-1',
  description: '',
  onDescriptionChange: vi.fn(),
  onOpenConversation: vi.fn(),
  onNewConversation: vi.fn(),
}

describe('AboutTab — new conversation', () => {
  it('renders a new conversation button', () => {
    render(<AboutTab {...baseProps} />)
    expect(screen.getByRole('button', { name: 'New conversation' })).toBeDefined()
  })

  it('calls onNewConversation when the + button is clicked', async () => {
    const onNewConversation = vi.fn()
    render(<AboutTab {...baseProps} onNewConversation={onNewConversation} />)
    await userEvent.click(screen.getByRole('button', { name: 'New conversation' }))
    expect(onNewConversation).toHaveBeenCalledOnce()
  })
})
