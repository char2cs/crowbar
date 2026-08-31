import { act, render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { HydrationGate } from '@/components/hydration-gate'

vi.mock('@/lib/persistence/hydrate', () => ({
  hydratePreferences: vi.fn().mockResolvedValue(null),
  hydrateSidebar: vi.fn().mockResolvedValue(null),
  hydrateWindowPaneLayout: vi.fn().mockResolvedValue(undefined),
}))

describe('HydrationGate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // boot() drives the workspace-list / project-data LoadableSlice fetches
    // through apiFetch. Stub the transport deterministically: without it the
    // fetches hit a rejecting network call which apiFetch now retries with real
    // backoff (the cold-start daemon-readiness fix), making the gate resolve too
    // slowly for the assertion. An empty-success envelope lets boot proceed fast.
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ success: true, data: [] }),
      })),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
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
