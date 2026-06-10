import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import userEvent from '@testing-library/user-event'
import { Button } from '@/components/ui/button'
import { TooltipProvider } from '@/components/ui/tooltip'

describe('Button', () => {
  it('renders a button element by default', () => {
    render(<Button>Click me</Button>)
    expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument()
  })

  it('applies ghost variant classes', () => {
    const { container } = render(<Button variant="ghost">Ghost</Button>)
    const btn = container.firstChild as HTMLElement
    expect(btn.className).toContain('hover:bg-accent')
  })

  it('renders tooltip content when tooltip prop is provided', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <Button tooltip="Go Back">Click me</Button>
      </TooltipProvider>,
    )
    await user.hover(screen.getByRole('button', { name: 'Click me' }))
    // Radix renders tooltip text twice in JSDOM — once visible, once in a hidden
    // accessibility node — so we use findAllByText and assert on the first match.
    expect((await screen.findAllByText('Go Back'))[0]).toBeInTheDocument()
  })

  it('renders Keybinding badge in tooltip when shortcut is provided', async () => {
    const user = userEvent.setup()
    render(
      <TooltipProvider>
        <Button tooltip="Close" shortcut="mod+w">
          X
        </Button>
      </TooltipProvider>,
    )
    await user.hover(screen.getByRole('button', { name: 'X' }))
    // Radix renders tooltip text twice in JSDOM — once visible, once in a hidden
    // accessibility node — so we use findAllByText and assert on the first match.
    expect((await screen.findAllByText('Close'))[0]).toBeInTheDocument()
    expect(document.querySelector('kbd')).toBeInTheDocument()
  })

  it('renders no tooltip when tooltip prop is absent', () => {
    render(<Button>Click me</Button>)
    expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument()
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  it('still accepts compact and commandId props without error (compat)', () => {
    expect(() =>
      render(
        <Button compact commandId="some.command">
          label
        </Button>,
      ),
    ).not.toThrow()
  })

  it('applies bg-accent/20 when active is true', () => {
    const { container } = render(<Button active>Active</Button>)
    const btn = container.firstChild as HTMLElement
    expect(btn.className).toContain('bg-accent/20')
  })

  it('shows spinner and sets data-loading when loading', () => {
    const { container } = render(<Button loading>Saving</Button>)
    const btn = container.firstChild as HTMLElement
    expect(btn).toHaveAttribute('data-loading', '')
    expect(container.querySelector('[data-slot="button-loading-indicator"]')).toBeInTheDocument()
  })

  it('is disabled when loading', () => {
    render(<Button loading>Saving</Button>)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('renders secondary variant as accent alias', () => {
    expect(() => render(<Button variant="secondary">Secondary</Button>)).not.toThrow()
  })

  it('renders ghost variant as muted alias', () => {
    expect(() => render(<Button variant="ghost">Ghost alias</Button>)).not.toThrow()
  })

  it('renders destructive variant as danger alias', () => {
    expect(() => render(<Button variant="destructive">Destructive</Button>)).not.toThrow()
  })
})
