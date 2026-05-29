import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkspaceLayoutRoot } from '@/features/workspace/components/WorkspaceLayoutRoot'
import { useUIState } from '@/features/window/stores/ui-state-store'

// Mock SplitViewRoot and BottomPane to avoid deep dependency chains
vi.mock('@/features/panes/components/split-view-root', () => ({
  SplitViewRoot: () => <div data-testid="split-view-root" />,
}))
vi.mock('@/features/workspace/components/bottom-pane', () => ({
  BottomPane: () => <div role="separator" aria-orientation="horizontal" />,
}))

describe('WorkspaceLayoutRoot', () => {
  it('does not render bottom pane when isBottomPaneVisible is false', () => {
    useUIState.setState({ isBottomPaneVisible: false })
    render(<WorkspaceLayoutRoot />)
    expect(screen.queryByRole('separator')).toBeNull()
  })

  it('renders bottom pane when isBottomPaneVisible is true', () => {
    useUIState.setState({ isBottomPaneVisible: true })
    render(<WorkspaceLayoutRoot />)
    expect(screen.getByRole('separator')).toBeTruthy()
  })
})
