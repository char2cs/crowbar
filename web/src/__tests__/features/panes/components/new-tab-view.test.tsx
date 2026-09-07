import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { NewTabView } from '@/features/panes/components/new-tab-view'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

// This used to offer New Terminal / New File / Review the Branch / a chat
// history list — real actions, each of which silently minted a brand-new,
// redundant chat for a pane that merely hadn't been told its workspace's real
// owning chat yet (the ensurePaneChatThenOpen bug — see
// pane-command-actions.test.ts). A pane with nothing open must never be a
// screen a user can act from, so these lock the fix in: nothing here is
// clickable, full stop.
describe('NewTabView', () => {
  it('renders no buttons at all', () => {
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })

  it('offers none of the old create actions', () => {
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    expect(screen.queryByText(/new terminal/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/new file/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/review the branch/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/new chat/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/untitled chat/i)).not.toBeInTheDocument()
  })

  it('renders no chat history rows or hand-off row', () => {
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    expect(screen.queryAllByTestId('nt-chat-row')).toHaveLength(0)
    expect(screen.queryByText(/more in this worktree/i)).not.toBeInTheDocument()
  })

  it('renders nothing focusable/interactive at all', () => {
    const { container } = render(<NewTabView paneId={ROOT_PANE_ID} />)
    expect(container.querySelectorAll('button, a, input, [role="button"]')).toHaveLength(0)
  })

  it('renders only the wordmark, marked decorative and inert', () => {
    const { container } = render(<NewTabView paneId={ROOT_PANE_ID} />)
    const svgs = container.querySelectorAll('svg')
    expect(svgs).toHaveLength(1)
    expect(svgs[0]).toHaveAttribute('aria-hidden', 'true')
    expect(svgs[0]).toHaveClass('pointer-events-none')
  })

  // The tumbling ASCII-art backdrop this pane used to lose along with the
  // real actions it correctly gave up (see the file-level comment above) —
  // ambient decoration, not a control, so it belongs back regardless.
  // Lazy-loaded (see new-tab-view.tsx's own note on why), so this waits for
  // the dynamic import to resolve rather than asserting synchronously.
  it('renders the tumbling ASCII-art backdrop, still marked decorative and inert', async () => {
    const { container } = render(<NewTabView paneId={ROOT_PANE_ID} />)
    const pre = await waitFor(() => {
      const el = container.querySelector('pre')
      expect(el).toBeInTheDocument()
      return el!
    })
    expect(pre.textContent).toBeTruthy()
    const backdrop = pre.closest('[aria-hidden="true"]')
    expect(backdrop).toHaveClass('pointer-events-none')
    // Still nothing clickable — the backdrop must not reopen the hole this
    // pane's whole simplification closed.
    expect(container.querySelectorAll('button, a, input, [role="button"]')).toHaveLength(0)
  })
})
