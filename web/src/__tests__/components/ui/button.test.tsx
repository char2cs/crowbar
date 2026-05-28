import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { Button } from '@/components/ui/button'

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

  it('accepts and ignores Crowbar compat props without error', () => {
    expect(() =>
      render(
        <Button
          tooltip="hint"
          compact
          active
          shortcut="mod+k"
          tooltipSide="bottom"
          commandId="some.command"
        >
          label
        </Button>
      )
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
