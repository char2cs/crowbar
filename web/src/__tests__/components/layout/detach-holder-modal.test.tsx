import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/lib/api/workspace', () => ({ detachHolder: vi.fn().mockResolvedValue({ items: [] }) }))

import { detachHolder } from '@/lib/api/workspace'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { DetachHolderModal } from '@/components/layout/detach-holder-modal'
import { useOperationsStore } from '@/lib/store/operations'

beforeEach(() => {
  vi.clearAllMocks()
  useDetachModalStore.setState({ target: null })
  useOperationsStore.setState({ ops: {}, latestFor: {} })
})

describe('DetachHolderModal', () => {
  it('renders nothing when there is no target', () => {
    const { container } = render(<DetachHolderModal />)
    expect(container).toBeEmptyDOMElement()
  })

  it('names the holder path and states files are safe', () => {
    useDetachModalStore.setState({
      target: { wsId: 'w1', branch: 'develop', heldByPath: '/Users/me/repo' },
    })
    render(<DetachHolderModal />)
    expect(screen.getByText(/\/Users\/me\/repo/)).toBeInTheDocument()
    expect(screen.getByText(/files are safe/i)).toBeInTheDocument()
  })

  it('Detach runs a detach_holder operation and closes', async () => {
    useDetachModalStore.setState({ target: { wsId: 'w1', branch: 'develop', heldByPath: '/repo' } })
    render(<DetachHolderModal />)
    await userEvent.click(screen.getByRole('button', { name: /^detach$/i }))
    const [op] = Object.values(useOperationsStore.getState().ops)
    expect(op).toMatchObject({
      kind: 'workspace.detach_holder',
      subject: { kind: 'workspace', id: 'w1' },
      label: 'Detach /repo',
    })
    expect(detachHolder).toHaveBeenCalledWith('w1', op.id)
    await waitFor(() => expect(useDetachModalStore.getState().target).toBeNull())
  })
})
