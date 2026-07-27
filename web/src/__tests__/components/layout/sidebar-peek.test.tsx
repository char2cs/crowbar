import { fireEvent, render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { SidebarPeek } from '@/components/layout/sidebar-peek'

vi.mock('@/utils/platform', () => ({ IS_MAC: true, IS_WINDOWS: false, IS_LINUX: false }))

const WIDTH = 320
// jsdom's viewport is 1024x768. The trigger band is 6px in from the edge,
// between the 44px chrome band and 16px above the bottom.
const RIGHT_EDGE_X = 1024 - 3
const SAFE_Y = 400

function host(): HTMLElement {
  return document.querySelector('[data-sidebar-peek]') as HTMLElement
}

function card(): HTMLElement {
  return document.querySelector('[data-sidebar-peek] > *') as HTMLElement
}

/**
 * The peek is hit-tested against the pointer, not served by an element.
 *
 * Dispatched as a MouseEvent rather than via `fireEvent.pointerMove`: jsdom does
 * not implement PointerEvent, so the coordinates the hit test depends on would
 * silently arrive undefined.
 */
function movePointer(clientX: number, clientY: number = SAFE_Y) {
  fireEvent(document, new MouseEvent('pointermove', { clientX, clientY, bubbles: true }))
}

function renderPeek(props: Partial<React.ComponentProps<typeof SidebarPeek>> = {}) {
  return render(
    <SidebarPeek hidden side="left" width={WIDTH} {...props}>
      <div data-testid="sidebar-body" />
    </SidebarPeek>,
  )
}

describe('SidebarPeek', () => {
  it('is an inert pass-through while the sidebar is docked', () => {
    renderPeek({ hidden: false })
    expect(host()).toHaveAttribute('data-state', 'docked')

    // Hovering the window edge of a DOCKED sidebar must summon nothing.
    movePointer(2)
    expect(host()).toHaveAttribute('data-state', 'docked')
  })

  it('peeks when the pointer reaches the edge and closes when it leaves', () => {
    renderPeek()
    expect(host()).toHaveAttribute('data-state', 'closed')

    movePointer(2)
    expect(host()).toHaveAttribute('data-state', 'peeking')

    movePointer(900)
    expect(host()).toHaveAttribute('data-state', 'closed')
  })

  it('stays open while the pointer is over the card, with no gap to cross', () => {
    renderPeek()
    movePointer(2)

    // Anywhere across the card's footprint plus its margin, contiguous with the
    // trigger band — this is what removes the need for a close timer.
    movePointer(WIDTH / 2)
    expect(host()).toHaveAttribute('data-state', 'peeking')

    movePointer(WIDTH + 8)
    expect(host()).toHaveAttribute('data-state', 'peeking')
  })

  it('does not summon the peek from the top chrome band or the bottom corner', () => {
    renderPeek()

    movePointer(2, 10)
    expect(host()).toHaveAttribute('data-state', 'closed')

    movePointer(2, 768 - 4)
    expect(host()).toHaveAttribute('data-state', 'closed')
  })

  it('mirrors the trigger to the right edge when docked right', () => {
    renderPeek({ side: 'right' })

    movePointer(2)
    expect(host()).toHaveAttribute('data-state', 'closed')

    movePointer(RIGHT_EDGE_X)
    expect(host()).toHaveAttribute('data-state', 'peeking')
    expect(card().className).toContain('right-2')
  })

  // The regression this whole component is shaped around: the sidebar subtree
  // must survive dock -> hide -> peek untouched. Rendering it into a separate
  // overlay container when hidden would rebuild the workspace tree, the
  // carousel scroll offset, the file explorer and the agent chat list.
  it('never rebuilds its children across docked, hidden and peeking', () => {
    const { rerender } = render(
      <SidebarPeek hidden={false} side="left" width={WIDTH}>
        <div data-testid="sidebar-body" />
      </SidebarPeek>,
    )
    const original = screen.getByTestId('sidebar-body')

    rerender(
      <SidebarPeek hidden side="left" width={WIDTH}>
        <div data-testid="sidebar-body" />
      </SidebarPeek>,
    )
    expect(screen.getByTestId('sidebar-body')).toBe(original)

    movePointer(2)
    expect(host()).toHaveAttribute('data-state', 'peeking')
    expect(screen.getByTestId('sidebar-body')).toBe(original)

    rerender(
      <SidebarPeek hidden={false} side="left" width={WIDTH}>
        <div data-testid="sidebar-body" />
      </SidebarPeek>,
    )
    expect(screen.getByTestId('sidebar-body')).toBe(original)
  })

  // Pinning the sidebar open while peeking used to strand `hovered` true, so the
  // next hide re-summoned the card under a pointer nowhere near it — and nothing
  // on screen could dismiss it.
  it('does not re-summon the card after being pinned open mid-peek', () => {
    const { rerender } = renderPeek()
    movePointer(2)
    expect(host()).toHaveAttribute('data-state', 'peeking')

    rerender(
      <SidebarPeek hidden={false} side="left" width={WIDTH}>
        <div data-testid="sidebar-body" />
      </SidebarPeek>,
    )
    expect(host()).toHaveAttribute('data-state', 'docked')

    rerender(
      <SidebarPeek hidden side="left" width={WIDTH}>
        <div data-testid="sidebar-body" />
      </SidebarPeek>,
    )
    expect(host()).toHaveAttribute('data-state', 'closed')
  })

  it('takes the parked card out of the tab order and puts it back on peek', () => {
    renderPeek()
    expect(card()).toHaveAttribute('inert')

    movePointer(2)
    expect(card()).not.toHaveAttribute('inert')
  })

  it('sizes the card to the remembered sidebar width', () => {
    renderPeek({ width: 451 })
    expect(card().style.getPropertyValue('--peek-width')).toBe('451px')
  })
})
