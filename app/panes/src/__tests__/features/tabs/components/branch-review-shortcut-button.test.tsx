import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { BranchReviewShortcutButton } from '@/features/tabs/components/branch-review-shortcut-button'

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
