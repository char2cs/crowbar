import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { PendingCreateRow } from '@/components/layout/pending-create-row'

describe('PendingCreateRow', () => {
  it('shows the branch name (with the in-flight spinner, no error) while creating', () => {
    render(
      <PendingCreateRow
        tempId="t1"
        pending={{ repoId: 'r1', parentId: 'd1', branch: 'feature/x' }}
        indent={14}
        onClear={() => {}}
      />,
    )
    expect(screen.getByText('feature/x')).toBeInTheDocument()
    // In-flight: no error/dismiss affordance is shown.
    expect(screen.queryByText('failed')).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('shows an inline error and dismisses via the ✕ button when the create failed', () => {
    const onClear = vi.fn()
    render(
      <PendingCreateRow
        tempId="t1"
        pending={{ repoId: 'r1', parentId: 'd1', branch: 'feature/x', error: 'boom' }}
        indent={14}
        onClear={onClear}
      />,
    )
    expect(screen.getByText('feature/x')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button'))
    expect(onClear).toHaveBeenCalledWith('t1')
  })
})
