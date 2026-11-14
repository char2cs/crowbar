import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const native = vi.hoisted(() => ({ on: false, confirm: vi.fn(async (): Promise<boolean> => true) }))
vi.mock('@/lib/native-chrome', () => ({
  hasNativeChrome: () => native.on,
  nativeConfirm: native.confirm,
}))
vi.mock('@/lib/api/workspace', () => ({ detachHolder: vi.fn().mockResolvedValue({ items: [] }) }))

import { detachHolder } from '@/lib/api/workspace'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { DetachHolderModal } from '@/components/layout/detach-holder-modal'
import { resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationsState } from '@/core/entities/operations'

beforeEach(() => {
  vi.clearAllMocks()
  native.on = false
  useDetachModalStore.setState({ target: null })
  resetOperations()
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
    const [op] = Object.values(operationsState().ops)
    expect(op).toMatchObject({
      kind: 'workspace.detach_holder',
      subject: { kind: 'workspace', id: 'w1' },
      label: 'Detach /repo',
    })
    expect(detachHolder).toHaveBeenCalledWith('w1', op.id)
    await waitFor(() => expect(useDetachModalStore.getState().target).toBeNull())
  })

  describe('on the phone', () => {
    const target = { wsId: 'w1', branch: 'develop', heldByPath: '/repo' }

    it('asks in a native alert instead of drawing a dialog, and detaches on yes', async () => {
      native.on = true
      useDetachModalStore.setState({ target })
      render(<DetachHolderModal />)
      expect(screen.queryByRole('dialog')).toBeNull()
      await waitFor(() =>
        expect(native.confirm).toHaveBeenCalledWith(
          expect.objectContaining({
            title: 'Detach to manage develop',
            confirmLabel: 'Detach',
            cancelLabel: 'Cancel',
            destructive: false,
          }),
        ),
      )
      await waitFor(() => expect(detachHolder).toHaveBeenCalledWith('w1', expect.any(String)))
      expect(useDetachModalStore.getState().target).toBeNull()
    })

    it('only closes on no', async () => {
      native.on = true
      native.confirm.mockResolvedValueOnce(false)
      useDetachModalStore.setState({ target })
      render(<DetachHolderModal />)
      await waitFor(() => expect(useDetachModalStore.getState().target).toBeNull())
      expect(detachHolder).not.toHaveBeenCalled()
    })
  })
})
