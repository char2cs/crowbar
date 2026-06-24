import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { PendingCreateRow } from '@/components/layout/pending-create-row'

// The in-flight spinner (@agilek/cli-loaders) reads window.matchMedia for the
// prefers-reduced-motion query, which jsdom does not provide.
beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

describe('PendingCreateRow', () => {
  it('shows the branch name (with the in-flight spinner, no error) while creating', () => {
    render(
      <PendingCreateRow
        tempId="t1"
        pending={{ repoId: 'r1', parentId: 'd1', branch: 'feature/x' }}
        paddingLeft={14}
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
        paddingLeft={14}
        onClear={onClear}
      />,
    )
    expect(screen.getByText('feature/x')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button'))
    expect(onClear).toHaveBeenCalledWith('t1')
  })
})
