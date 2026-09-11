import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PaneTopRow } from '@/features/tabs/components/pane-top-row'

function renderRow(overrides: Partial<React.ComponentProps<typeof PaneTopRow>> = {}) {
  const rowRef = createRef<HTMLDivElement>()
  return render(
    <PaneTopRow
      rowRef={rowRef}
      isBottomPane={false}
      isAtLeftEdge={false}
      isAtTopEdge={false}
      {...overrides}
    >
      content
    </PaneTopRow>,
  )
}

// The shared shell TabBar, ChatOnlyPaneHeader, and the side-by-side/stacked
// chat header all render through — one place owning the window-chrome
// contract (drag region, macOS traffic-light inset) so it can never drift
// between "kinds" of pane top row. Edge geometry is a PROP, not measured
// internally, so a caller that already has its own `usePaneTopRowEdges()`
// call (TabBar's `useTabBarScroll`) never ends up with two observers on the
// same element.
describe('PaneTopRow', () => {
  it('carries the window drag region', () => {
    renderRow()
    expect(screen.getByTestId('pane-top-row')).toHaveAttribute('data-tauri-drag-region')
  })

  it('renders its children', () => {
    render(
      <PaneTopRow
        rowRef={createRef()}
        isBottomPane={false}
        isAtLeftEdge={false}
        isAtTopEdge={false}
      >
        <span>hello</span>
      </PaneTopRow>,
    )
    expect(screen.getByText('hello')).toBeInTheDocument()
  })

  it('merges a caller-supplied className (e.g. the background token)', () => {
    renderRow({ className: 'bg-chrome-bg' })
    expect(screen.getByTestId('pane-top-row')).toHaveClass('bg-chrome-bg')
  })
})
