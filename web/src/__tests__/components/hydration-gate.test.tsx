import { act, render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { HydrationGate } from '@/components/hydration-gate'

vi.mock('@/lib/persistence/hydrate', () => ({
  hydrateFromIDB: vi.fn().mockResolvedValue({
    layout: null,
    prefs: null,
    editorStates: [],
  }),
}))

describe('HydrationGate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders children after hydration resolves', async () => {
    const { hydrateFromIDB } = await import('@/lib/persistence/hydrate')
    vi.mocked(hydrateFromIDB).mockResolvedValue({ layout: null, prefs: null, editorStates: [] })

    await act(async () => {
      render(
        <HydrationGate workspaceId="ws-1">
          <div>app content</div>
        </HydrationGate>
      )
    })

    await waitFor(() => {
      expect(screen.getByText('app content')).toBeInTheDocument()
    })
  })

  it('renders nothing before hydration resolves', async () => {
    // Use a never-resolving promise so the gate stays closed
    const { hydrateFromIDB } = await import('@/lib/persistence/hydrate')
    vi.mocked(hydrateFromIDB).mockImplementation(() => new Promise(() => {}))

    const { container } = render(
      <HydrationGate workspaceId="ws-pending">
        <span>should not appear</span>
      </HydrationGate>
    )

    expect(screen.queryByText('should not appear')).toBeNull()
    expect(container.firstChild).toBeNull()
  })
})
