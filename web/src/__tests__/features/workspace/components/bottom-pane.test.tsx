import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, fireEvent, cleanup } from '@testing-library/react'
import { BottomPane } from '@/features/workspace/components/bottom-pane'
import { useUIState } from '@/features/window/stores/ui-state-store'

// Mock deep dependencies
vi.mock('@/features/workspace/stores/hooks/use-pane-store', () => ({
  useBottomRoot: () => ({ type: 'group', id: 'bottom-pane', bufferIds: [], activeBufferId: null }),
}))
vi.mock('@/features/panes/components/pane-node-renderer', () => ({
  PaneNodeRenderer: () => <div data-testid="pane-node-renderer" />,
}))

describe('BottomPane', () => {
  afterEach(() => {
    cleanup()
    useUIState.setState({ bottomPaneHeight: 300 })
  })

  it('renders a resize handle', () => {
    const { getByRole } = render(<BottomPane />)
    expect(getByRole('separator')).toBeTruthy()
  })

  it('clamps dragged height to maximum of 600', () => {
    useUIState.setState({ bottomPaneHeight: 300 })
    const { getByRole } = render(<BottomPane />)
    const handle = getByRole('separator')

    fireEvent.mouseDown(handle, { clientY: 200 })
    // Move up by 1000px (way more than max)
    fireEvent.mouseMove(document, { clientY: -800 })
    fireEvent.mouseUp(document)

    // Height should be clamped to 600
    expect(useUIState.getState().bottomPaneHeight).toBeLessThanOrEqual(600)
  })

  it('clamps dragged height to minimum of 120', () => {
    useUIState.setState({ bottomPaneHeight: 300 })
    const { getByRole } = render(<BottomPane />)
    const handle = getByRole('separator')

    fireEvent.mouseDown(handle, { clientY: 200 })
    // Move down by 1000px (way more than min)
    fireEvent.mouseMove(document, { clientY: 1200 })
    fireEvent.mouseUp(document)

    // Height should be clamped to 120
    expect(useUIState.getState().bottomPaneHeight).toBeGreaterThanOrEqual(120)
  })
})
