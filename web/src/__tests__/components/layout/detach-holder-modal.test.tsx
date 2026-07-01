import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/lib/api/workspace', () => ({ detachHolder: vi.fn().mockResolvedValue(undefined) }))

import { detachHolder } from '@/lib/api/workspace'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { DetachHolderModal } from '@/components/layout/detach-holder-modal'

beforeEach(() => {
  vi.clearAllMocks()
  useDetachModalStore.setState({ target: null })
})

describe('DetachHolderModal', () => {
  it('renders nothing when there is no target', () => {
    const { container } = render(<DetachHolderModal />)
    expect(container).toBeEmptyDOMElement()
  })

  it('names the holder path and states files are safe', () => {
    useDetachModalStore.setState({ target: { wsId: 'w1', branch: 'develop', heldByPath: '/Users/me/repo' } })
    render(<DetachHolderModal />)
    expect(screen.getByText(/\/Users\/me\/repo/)).toBeInTheDocument()
    expect(screen.getByText(/files are safe/i)).toBeInTheDocument()
  })

  it('detaches and closes on confirm', async () => {
    useDetachModalStore.setState({ target: { wsId: 'w1', branch: 'develop', heldByPath: '/repo' } })
    render(<DetachHolderModal />)
    await userEvent.click(screen.getByRole('button', { name: /^detach$/i }))
    expect(detachHolder).toHaveBeenCalledWith('w1')
    await waitFor(() => expect(useDetachModalStore.getState().target).toBeNull())
  })
})
