import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { StaleBanner } from '@/components/ui/stale-banner'

describe('StaleBanner', () => {
  it('shows cached label and a retry control when not refreshing', () => {
    const onRetry = vi.fn()
    render(<StaleBanner at={Date.now() - 60_000} onRetry={onRetry} isRefreshing={false} />)
    expect(screen.getByText(/cached data/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('shows Refreshing… and no retry button while refreshing', () => {
    render(<StaleBanner at={Date.now()} onRetry={() => {}} isRefreshing />)
    expect(screen.getByText(/refreshing/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull()
  })
})
