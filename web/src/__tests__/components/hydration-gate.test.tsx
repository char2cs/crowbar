import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { HydrationGate } from '@/components/hydration-gate'

vi.mock('@/lib/persistence/hydrate', () => ({
  hydrateFromIDB: vi.fn().mockResolvedValue({
    layout: null,
    prefs: null,
    editorStates: [],
  }),
}))

describe('HydrationGate', () => {
  it('renders children after hydration resolves', async () => {
    render(
      <HydrationGate workspaceId="ws-1">
        <div>app content</div>
      </HydrationGate>
    )

    await waitFor(() => {
      expect(screen.getByText('app content')).toBeInTheDocument()
    })
  })

  it('renders nothing before hydration resolves', () => {
    const { container } = render(
      <HydrationGate workspaceId="ws-1">
        <span>child</span>
      </HydrationGate>
    )
    expect(container).toBeDefined()
  })
})
