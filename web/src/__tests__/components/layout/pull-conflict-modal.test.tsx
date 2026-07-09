import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { usePullConflictModalStore } from '@/features/git/stores/use-pull-conflict-modal-store'
import { PullConflictModal } from '@/components/layout/pull-conflict-modal'

beforeEach(() => {
  usePullConflictModalStore.setState({ target: null })
})

describe('PullConflictModal', () => {
  it('renders nothing when there is no target', () => {
    const { container } = render(<PullConflictModal />)
    expect(container).toBeEmptyDOMElement()
  })

  it('names the branch and explains the divergence when opened', () => {
    usePullConflictModalStore.getState().open({ wsId: 'w1', branch: 'feature/thing' })
    render(<PullConflictModal />)
    expect(screen.getByText(/branch has diverged/i)).toBeInTheDocument()
    expect(screen.getByText(/feature\/thing/)).toBeInTheDocument()
    // Inform-only: no merge/rebase action, just an acknowledgement.
    expect(screen.getByRole('button', { name: /got it/i })).toBeInTheDocument()
  })

  it('closes on the "Got it" button', async () => {
    usePullConflictModalStore.getState().open({ wsId: 'w1', branch: 'develop' })
    render(<PullConflictModal />)
    await userEvent.click(screen.getByRole('button', { name: /got it/i }))
    await waitFor(() => expect(usePullConflictModalStore.getState().target).toBeNull())
  })
})
