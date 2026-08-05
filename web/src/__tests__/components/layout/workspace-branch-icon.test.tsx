import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'

describe('WorkspaceBranchIcon', () => {
  it('renders the centralized flip-dot spinner when working', () => {
    const { container, getByRole } = render(<WorkspaceBranchIcon status="new" working />)
    const status = getByRole('status')
    expect(status).toBeTruthy()
    // Flicker spinner, not the retired @agilek/cli-loaders Spinner.
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
