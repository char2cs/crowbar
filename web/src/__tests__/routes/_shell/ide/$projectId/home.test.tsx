import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => (config: unknown) => config,
  useParams: () => ({ projectId: 'p1' }),
}))

const { useHomeWorkspaceStateMock } = vi.hoisted(() => ({
  useHomeWorkspaceStateMock: vi.fn(),
}))

vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  useHomeWorkspaceState: useHomeWorkspaceStateMock,
}))

import { HomeRoute } from '@/routes/_shell/ide/$projectId/home'

// HomeRoute no longer renders WorkspaceView itself (that used to mean a fresh
// fetch+hydrate+mount, and a full store teardown, on every single visit to
// project home). IDEShell now resolves the home workspace id once per project
// (home-workspace-resolver.ts) and keeps its WorkspaceView mounted-but-hidden
// in WorkspaceHost, the same keep-alive retention real workspaces get — see
// ide-shell.tsx. This route only surfaces the loading/error state.
describe('HomeRoute component', () => {
  it('renders nothing while the home workspace id is still resolving', () => {
    useHomeWorkspaceStateMock.mockReturnValue({ wsId: null, error: false })
    const { container } = render(<HomeRoute />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing once resolved — WorkspaceHost (an Outlet sibling in IDEShell) paints the content', () => {
    useHomeWorkspaceStateMock.mockReturnValue({ wsId: 'ws-home-1', error: false })
    const { container } = render(<HomeRoute />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders fallback on error', () => {
    useHomeWorkspaceStateMock.mockReturnValue({ wsId: null, error: true })
    render(<HomeRoute />)
    expect(screen.getByText(/unavailable/i)).toBeInTheDocument()
  })
})
