import { render, cleanup } from '@testing-library/react'
import { describe, it, expect, afterEach, vi } from 'vitest'
import { Toolbar, ToolbarButton } from '@/components/ui/toolbar'
import { TooltipProvider } from '@/components/ui/tooltip'

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
    const { container } = renderToolbar(
      <ToolbarButton tooltip="Bold (⌘+B)" pressed={true}>
        <span>B</span>
      </ToolbarButton>,
    )

    const button = container.querySelector('button')
    expect(button).not.toBeNull()
    expect(button!.getAttribute('aria-pressed')).toBe('true')
  })
})
