import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => (config: unknown) => config,
  useParams: () => ({ projectId: 'p1' }),
}))

vi.mock('@/lib/api', () => ({
  fetchHomeWorkspace: vi.fn(),
}))

vi.mock('@/features/workspace/components/workspace-view', () => ({
  WorkspaceView: ({ wsId }: { wsId: string }) => <div data-testid="workspace-view">{wsId}</div>,
}))

import { fetchHomeWorkspace } from '@/lib/api'
import { HomeRoute } from '@/routes/_shell/ide/$projectId/home'

beforeEach(() => {
  vi.mocked(fetchHomeWorkspace).mockClear()
})

describe('HomeRoute component', () => {
  it('renders WorkspaceView with the home workspace id on success', async () => {
    vi.mocked(fetchHomeWorkspace).mockResolvedValueOnce({
      id: 'ws-home-1',
      projectId: 'p1',
      kind: 'home',
    } as never)

    render(<HomeRoute />)

    await waitFor(() => {
      expect(screen.getByTestId('workspace-view')).toBeInTheDocument()
      expect(screen.getByTestId('workspace-view').textContent).toBe('ws-home-1')
    })
  })

  it('renders loading state initially', () => {
    vi.mocked(fetchHomeWorkspace).mockReturnValue(new Promise(() => {}))
    render(<HomeRoute />)
    expect(screen.queryByTestId('workspace-view')).not.toBeInTheDocument()
  })

  it('renders fallback on error', async () => {
    vi.mocked(fetchHomeWorkspace).mockRejectedValueOnce(new Error('not found'))
    render(<HomeRoute />)
    await waitFor(() => {
      expect(screen.getByText(/unavailable/i)).toBeInTheDocument()
    })
  })
})
