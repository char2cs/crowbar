import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'

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
