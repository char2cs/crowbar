import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'

describe('WorkspaceBranchIcon', () => {
  it('renders the centralized flip-dot spinner when working', () => {
    const { container, getByRole } = render(<WorkspaceBranchIcon status="new" working />)
    const status = getByRole('status')
    expect(status).toBeTruthy()
    // The flicker spinner.
    expect(container.querySelector('[data-flicker-spinner]')).not.toBeNull()
    // Theme-token colored, never hardcoded — the wrapper carries the color class
    // since FlickerSpinner inherits via currentColor.
    expect(container.querySelector('.text-foreground')).not.toBeNull()
  })

  it('renders the branch glyph when idle', () => {
    const { queryByRole } = render(<WorkspaceBranchIcon status="new" />)
    expect(queryByRole('status')).toBeNull()
  })
})

describe('WorkspaceBranchIcon placeholder', () => {
  it('renders the warning glyph (not the lock glyph) for a placeholder', () => {
    render(<WorkspaceBranchIcon status="locked" isPlaceholder />)
    expect(screen.getByRole('img', { name: /needs provisioning/i })).toBeInTheDocument()
  })

  it('renders the lock glyph for a healthy locked workspace', () => {
    render(<WorkspaceBranchIcon status="locked" />)
    expect(screen.queryByRole('img', { name: /needs provisioning/i })).toBeNull()
  })
})

// On the inverted TAB_ACTIVE ground, `invertedGround` swaps the ambient `text-foreground` so the
// icon stays visible; the fixed status colours read on either ground and are untouched.
describe('WorkspaceBranchIcon invertedGround', () => {
  it('swaps text-foreground for text-foreground-inverse on a "new" branch icon', () => {
    const { container } = render(<WorkspaceBranchIcon status="new" invertedGround />)
    const icon = container.querySelector('svg')!
    expect(icon).toHaveClass('text-foreground-inverse')
    expect(icon).not.toHaveClass('text-foreground')
  })

  it('swaps text-foreground for text-foreground-inverse on a locked branch icon', () => {
    const { container } = render(<WorkspaceBranchIcon status="locked" invertedGround />)
    const icon = container.querySelector('svg')!
    expect(icon).toHaveClass('text-foreground-inverse')
    expect(icon).not.toHaveClass('text-foreground')
  })

  it('does not touch the fixed status colors', () => {
    const { container } = render(<WorkspaceBranchIcon status="pr-open" invertedGround />)
    expect(container.querySelector('svg')).toHaveClass('text-green-500')
  })

  it('leaves text-foreground alone when not on an inverted ground', () => {
    const { container } = render(<WorkspaceBranchIcon status="new" />)
    const icon = container.querySelector('svg')!
    expect(icon).toHaveClass('text-foreground')
    expect(icon).not.toHaveClass('text-foreground-inverse')
  })
})
