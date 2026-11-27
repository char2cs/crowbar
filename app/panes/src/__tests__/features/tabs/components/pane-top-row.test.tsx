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

    it('stays in normal flex flow by default — overlay is a SEPARATE, opt-in prop', () => {
      renderRow({ variant: 'chat-blur' })
      expect(screen.getByTestId('pane-top-row')).not.toHaveClass('absolute')
    })
  })

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
