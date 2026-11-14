import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { BranchReviewShortcutButton } from '@/features/tabs/components/branch-review-shortcut-button'

// A shortcut to the SAME "Review this branch" action GitPanel's own button
// triggers (git-panel.tsx) — this never replaces that entry point, it's an
// extra way to reach it from the IDE sector's own tab row.
describe('BranchReviewShortcutButton', () => {
  it('fires onOpen when clicked', () => {
    const onOpen = vi.fn()
    render(<BranchReviewShortcutButton isBottomPane={false} onOpen={onOpen} />)
    fireEvent.click(screen.getByRole('button', { name: /review this branch/i }))
    expect(onOpen).toHaveBeenCalledTimes(1)
  })

  it('renders nothing on the bottom pane, like its row neighbours', () => {
    render(<BranchReviewShortcutButton isBottomPane={true} onOpen={() => {}} />)
    expect(screen.queryByRole('button', { name: /review this branch/i })).not.toBeInTheDocument()
  })
})
