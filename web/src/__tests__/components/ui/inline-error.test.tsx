import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { InlineError } from '@/components/ui/inline-error'

describe('InlineError', () => {
  it('shows the failure heading and calls onRetry', () => {
    const onRetry = vi.fn()
    render(<InlineError error={new Error('nope')} onRetry={onRetry} />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(onRetry).toHaveBeenCalledOnce()
  })
})
