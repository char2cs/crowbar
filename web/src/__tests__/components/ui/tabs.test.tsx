import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { Tab } from '@/components/ui/tabs'

// Regression: a mouse click on a tab (ChatHead in particular, tab-bar.tsx)
// left it visibly wearing the `:focus-visible` ring for as long as focus
// stayed there — WebKit shows that ring on a plain click, Chromium doesn't,
// so this only ever showed up live. Caught as "the chat head flashes with
// something the instant I click another pane" — the ring appearing (and,
// once you click elsewhere, disappearing) is a real UI artifact even though
// no data/content ever changed. Fixed by preventing the DEFAULT
// focus-follows-mousedown behavior on Tab's own button — keyboard Tab
// navigation never goes through mousedown, so it is unaffected.
describe('Tab — a mouse click never leaves it visibly focused', () => {
  it('prevents the mousedown default (the one thing that would move focus there)', () => {
    render(<Tab>Athas test</Tab>)
    const button = screen.getByRole('button', { name: 'Athas test' })

    // fireEvent returns the DOM dispatchEvent result: false iff some handler
    // called preventDefault() on a cancelable event.
    const notPrevented = fireEvent.mouseDown(button)
    expect(notPrevented).toBe(false)
  })

  it("still runs the caller's own onMouseDown (dnd-kit's drag activation on an editor tab)", () => {
    const onMouseDown = vi.fn()
    render(<Tab onMouseDown={onMouseDown}>Athas test</Tab>)
    const button = screen.getByRole('button', { name: 'Athas test' })

    fireEvent.mouseDown(button)

    expect(onMouseDown).toHaveBeenCalledTimes(1)
  })

  it("defers to the caller's own preventDefault() call rather than double-handling it", () => {
    const onMouseDown = vi.fn((e: React.MouseEvent) => e.preventDefault())
    render(<Tab onMouseDown={onMouseDown}>Athas test</Tab>)
    const button = screen.getByRole('button', { name: 'Athas test' })

    const notPrevented = fireEvent.mouseDown(button)

    expect(onMouseDown).toHaveBeenCalledTimes(1)
    expect(notPrevented).toBe(false)
  })

  it('a click still activates the tab — preventing mousedown default never blocks the click', () => {
    const onClick = vi.fn()
    render(<Tab onClick={onClick}>Athas test</Tab>)
    const button = screen.getByRole('button', { name: 'Athas test' })

    fireEvent.mouseDown(button)
    fireEvent.mouseUp(button)
    fireEvent.click(button)

    expect(onClick).toHaveBeenCalledTimes(1)
  })
})
