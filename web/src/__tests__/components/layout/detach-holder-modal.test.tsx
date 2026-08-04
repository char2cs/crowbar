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
    useDetachModalStore.setState({
      target: { wsId: 'w1', branch: 'develop', heldByPath: '/Users/me/repo' },
    })
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

  // Regression for `native/oracle/blocked/repo-import-dialog-duplicate-button-id.md`:
  // this modal's two footer buttons (Cancel, Detach) and the Dialog
  // primitive's built-in close button all used to inherit `button.tsx`'s
  // default `data-oracle-id="button"`, so a capture rooted at this modal
  // carried three `button`-id anchors and the oracle differ refused it
  // outright. The close button now names itself `dialog-close`.
  //
  // Mutation actually run (`data-oracle-id="dialog-close"` deleted from
  // `dialog.tsx`'s `DialogPopup` close button, then reverted):
  //
  //   AssertionError: expected …(3) to have a length of 2 but got 3
  //   - Expected: 2
  //   + Received: 3
  //   at expect(document.querySelectorAll('[data-oracle-id="button"]')).toHaveLength(2)
  //
  // Three `button`-id anchors (Cancel, Detach, and the close button) —
  // exactly the collision the blocked doc names for this surface.
  it('the close button does not collide with the two body Buttons on data-oracle-id', () => {
    useDetachModalStore.setState({ target: { wsId: 'w1', branch: 'develop', heldByPath: '/repo' } })
    render(<DetachHolderModal />)

    expect(document.querySelectorAll('[data-oracle-id="button"]')).toHaveLength(2)
    expect(document.querySelectorAll('[data-oracle-id="dialog-close"]')).toHaveLength(1)
  })
})
