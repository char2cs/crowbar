import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { Tab } from '@/components/ui/tabs'

describe('Tab — a mouse click never leaves it visibly focused', () => {
  it('prevents the mousedown default (the one thing that would move focus there)', () => {
    render(<Tab>Athas test</Tab>)
    const button = screen.getByRole('button', { name: 'Athas test' })

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

describe('Tab ghost variant', () => {
  it('is transparent with muted text at rest', () => {
    render(
      <Tab variant="ghost" isActive={false}>
        File.ts
      </Tab>,
    )
    const tab = screen.getByRole('button', { name: 'File.ts' })
    expect(tab).toHaveClass('border-transparent')
    expect(tab).toHaveClass('text-muted-foreground')
    expect(tab).not.toHaveClass('bg-sidebar-element-hover')
  })

  it('shows a persistent sidebar-element-hover fill and foreground text when active', () => {
    render(
      <Tab variant="ghost" isActive>
        File.ts
      </Tab>,
    )
    const tab = screen.getByRole('button', { name: 'File.ts' })
    expect(tab).toHaveClass('bg-sidebar-element-hover')
    expect(tab).toHaveClass('text-foreground')
  })

  it('never renders the underline-variant bar, even when active', () => {
    render(
      <Tab variant="ghost" isActive>
        File.ts
      </Tab>,
    )
    expect(screen.queryByTestId('tab-underline')).not.toBeInTheDocument()
  })

  it('never renders the pill variant fill/rounding', () => {
    render(
      <Tab variant="ghost" isActive>
        File.ts
      </Tab>,
    )
    const tab = screen.getByRole('button', { name: 'File.ts' })
    expect(tab).not.toHaveClass('rounded-full')
    expect(tab).not.toHaveClass('bg-background')
  })
})
