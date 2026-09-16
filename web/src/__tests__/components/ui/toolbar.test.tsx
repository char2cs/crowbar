import { render, cleanup } from '@testing-library/react'
import { describe, it, expect, afterEach, vi } from 'vitest'
import { Toolbar, ToolbarButton } from '@/components/ui/toolbar'
import { TooltipProvider } from '@/components/ui/tooltip'

/**
 * Live-observed in the running app, on the chat composer's selection
 * formatting toolbar (`ChatFloatingToolbarKit` -> `MarkToolbarButton` ->
 * `ToolbarButton`), the moment Bold rendered:
 *
 *   In HTML, <button> cannot be a descendant of <button>.
 *   This will cause a hydration error.
 *
 * `withTooltip` wrapped the button in a Radix `Tooltip.Trigger` that had lost
 * its `asChild` (the upstream Plate registry ships it), so the trigger
 * rendered a <button> of its own around a component that already resolves to
 * one — `Toolbar.ToggleItem` on the `pressed` branch, `Toolbar.Button`
 * otherwise. Both branches are covered below; a tooltip must never add an
 * element of its own to a toolbar button.
 */
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function renderToolbar(ui: React.ReactNode) {
  const errors = vi.spyOn(console, 'error').mockImplementation(() => {})
  const { container } = render(
    <TooltipProvider>
      <Toolbar>{ui}</Toolbar>
    </TooltipProvider>,
  )
  return { container, errors }
}

function nestingComplaints(calls: unknown[][]): unknown[][] {
  return calls.filter((call) =>
    call.some((arg) => typeof arg === 'string' && /cannot (be a descendant of|contain)/.test(arg)),
  )
}

describe('ToolbarButton + tooltip', () => {
  it('renders a toggle button with no nested <button> and no DOM-nesting warning', () => {
    const { container, errors } = renderToolbar(
      <ToolbarButton tooltip="Bold (⌘+B)" pressed={false}>
        <span data-testid="icon">B</span>
      </ToolbarButton>,
    )

    expect(container.querySelector('button button')).toBeNull()
    expect(container.querySelectorAll('button')).toHaveLength(1)
    expect(nestingComplaints(errors.mock.calls)).toEqual([])
  })

  it('renders a plain (non-toggle) button with no nested <button> either', () => {
    const { container, errors } = renderToolbar(
      <ToolbarButton tooltip="Link (⌘+K)">
        <span>L</span>
      </ToolbarButton>,
    )

    expect(container.querySelector('button button')).toBeNull()
    expect(container.querySelectorAll('button')).toHaveLength(1)
    expect(nestingComplaints(errors.mock.calls)).toEqual([])
  })

  it('still describes the button with the tooltip once the trigger is wired to it', () => {
    // The point of `asChild`: the trigger's aria/state props land on the REAL
    // button, not on a wrapper around it.
    const { container } = renderToolbar(
      <ToolbarButton tooltip="Bold (⌘+B)" pressed={true}>
        <span>B</span>
      </ToolbarButton>,
    )

    const button = container.querySelector('button')
    expect(button).not.toBeNull()
    expect(button!.getAttribute('data-state')).not.toBeNull()
  })
})
