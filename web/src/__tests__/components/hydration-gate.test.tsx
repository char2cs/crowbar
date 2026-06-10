import { act, render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { HydrationGate } from '@/components/hydration-gate'

vi.mock('@/lib/persistence/hydrate', () => ({
  hydratePreferences: vi.fn().mockResolvedValue(null),
  hydrateSidebar: vi.fn().mockResolvedValue(null),
}))

describe('HydrationGate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders children after preferences hydration resolves', async () => {
    const { hydratePreferences } = await import('@/lib/persistence/hydrate')
    vi.mocked(hydratePreferences).mockResolvedValue(null)

    await act(async () => {
      render(
        <HydrationGate>
          <div>app content</div>
        </HydrationGate>,
      )
    })

    await waitFor(() => {
      expect(screen.getByText('app content')).toBeInTheDocument()
    })
  })

  it('renders nothing before hydration resolves', async () => {
    const { hydratePreferences } = await import('@/lib/persistence/hydrate')
    vi.mocked(hydratePreferences).mockImplementation(() => new Promise(() => {}))

    const { container } = render(
      <HydrationGate>
        <span>should not appear</span>
      </HydrationGate>,
    )

    expect(screen.queryByText('should not appear')).toBeNull()
    expect(container.firstChild).toBeNull()
  })
})
