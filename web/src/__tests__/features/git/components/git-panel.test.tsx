import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { GitPanel } from '@/features/git/components/git-panel'

vi.mock('@/features/git/components/git-changes-panel', () => ({
  GitChangesPanel: () => <div data-testid="git-changes" />,
}))
vi.mock('@/features/git/components/git-history-list', () => ({
  GitHistoryList: () => <div data-testid="git-history" />,
}))

describe('GitPanel', () => {
  it('renders Changes and History tabs', () => {
    render(<GitPanel />)
    expect(screen.getByRole('tab', { name: /changes/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /history/i })).toBeInTheDocument()
  })

  it('shows the changes panel in the Changes tab by default', () => {
    render(<GitPanel />)
    expect(screen.getByTestId('git-changes')).toBeInTheDocument()
  })
})
