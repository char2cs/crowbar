import { describe, it, expect } from 'vitest'
import type { PaneContent } from '@/features/panes/types/pane-content'

function shouldShowCloseButton(buffer: PaneContent): boolean {
  return !buffer.isUncloseable
}

describe('tab close button visibility', () => {
  it('shows close button for normal editor tabs', () => {
    expect(shouldShowCloseButton({ type: 'editor', isUncloseable: undefined } as unknown as PaneContent)).toBe(true)
  })

  it('hides close button for branchReview tabs', () => {
    expect(shouldShowCloseButton({ type: 'branchReview', isUncloseable: true } as unknown as PaneContent)).toBe(false)
  })
})
