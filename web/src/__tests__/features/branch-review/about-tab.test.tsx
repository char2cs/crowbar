import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { success, failed, idle } from '@/lib/loadable'
import { useBranchReviewDataStore } from '@/features/branch-review/stores/branch-review-data-store'
import { AboutTab } from '@/features/branch-review/components/about-tab'

const data = (chats: { id: string; title: string; age: string; isActive: boolean }[]) =>
  ({ diff: { files: [] }, chats })

beforeEach(() => { useBranchReviewDataStore.setState({ data: idle() }) })

const noop = () => {}

describe('AboutTab conversations', () => {
  it('renders chats on success', () => {
    useBranchReviewDataStore.setState({ data: success(data([{ id: 'c1', title: 'Arch review', age: '2h', isActive: true }])) })
    render(<AboutTab wsId="ws3" description="" onDescriptionChange={noop} onOpenConversation={noop} />)
    expect(screen.getByText('Arch review')).toBeInTheDocument()
  })

  it('shows inline error when chats fail with no cache', () => {
    useBranchReviewDataStore.setState({ data: failed(new Error('500'), idle()) })
    render(<AboutTab wsId="ws3" description="" onDescriptionChange={noop} onOpenConversation={noop} />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
  })
})
