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

  it('merges a caller-supplied className', () => {
    renderRow({ className: 'shrink-0' })
    expect(screen.getByTestId('pane-top-row')).toHaveClass('shrink-0')
  })

  // variant="opaque" (default) — the IDE sector's own look: a flat,
  // fully-opaque fill, sitting normally in flex flow above whatever it
  // heads (nothing needs to render "behind" it).
  describe('variant="opaque" (default)', () => {
    it('paints the opaque pane-background fill', () => {
      renderRow()
      expect(screen.getByTestId('pane-top-row')).toHaveClass('bg-pane-background')
    })

    it('renders no dissolve overlay', () => {
      renderRow()
      expect(screen.queryByTestId('edge-dissolve')).not.toBeInTheDocument()
    })

    it('stays in normal flex flow — not positioned as a floating overlay', () => {
      renderRow()
      expect(screen.getByTestId('pane-top-row')).not.toHaveClass('absolute')
    })
  })

  // variant="chat-blur" — the chat's own glass: no fill of its own, steals
  // the composer's own progressive-blur "dissolve" so scrolled-behind text
  // blurs and fades instead of being clipped by a hard edge.
  describe('variant="chat-blur"', () => {
    it('paints no opaque fill of its own', () => {
      renderRow({ variant: 'chat-blur' })
      expect(screen.getByTestId('pane-top-row')).not.toHaveClass('bg-pane-background')
    })

    it('renders the dissolve UNDER its own content, both anchored to the top edge', () => {
      renderRow({ variant: 'chat-blur' })
      expect(screen.getByTestId('edge-dissolve')).toHaveAttribute('data-edge', 'top')
      expect(screen.getByText('content')).toBeInTheDocument()
    })

    // TabBar's own row uses chat-blur's LOOK without `overlay` — it sits
    // outside the chat/editor split entirely, so floating it would resize
    // that split's container out from under it, not just change its look.
    it('stays in normal flex flow by default — overlay is a SEPARATE, opt-in prop', () => {
      renderRow({ variant: 'chat-blur' })
      expect(screen.getByTestId('pane-top-row')).not.toHaveClass('absolute')
    })
  })

  // overlay — floats the row free of flex flow so a caller that lives
  // INSIDE the box whose content should show through (ChatColumnHeader,
  // ChatOnlyPaneHeader) can let that content fill the full box behind it.
  describe('overlay', () => {
    it('floats as an absolute overlay pinned to the top edge', () => {
      renderRow({ overlay: true })
      const row = screen.getByTestId('pane-top-row')
      expect(row).toHaveClass('absolute', 'top-0')
    })

    it('works independently of variant — an opaque row can overlay too', () => {
      renderRow({ overlay: true, variant: 'opaque' })
      const row = screen.getByTestId('pane-top-row')
      expect(row).toHaveClass('absolute', 'bg-pane-background')
    })
  })
})
