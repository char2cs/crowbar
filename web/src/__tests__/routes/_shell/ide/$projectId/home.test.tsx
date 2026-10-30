import { render, screen } from '@testing-library/react'
import { afterEach, describe, it, expect, vi } from 'vitest'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

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

  it('renders fallback on error, saying what to do', () => {
    useHomeWorkspaceStateMock.mockReturnValue({ wsId: null, error: true })
    render(<HomeRoute />)
    expect(screen.getByText(/unavailable/i)).toBeInTheDocument()
    expect(screen.getByText(/connect to this project's host/i)).toBeInTheDocument()
  })

  // The route stays while the host the project may be on is down; it must not stay blank.
  describe('on an install that paired a remote host', () => {
    afterEach(() => {
      resetHostWorld()
      destroyHostRuntime('h1')
    })

    it('says Project Home is unavailable while the only host that may hold it is down', () => {
      pairRemoteHost('h1')
      getOrCreateHostRuntime('h1').getState().setStatus('down')
      loadProjects(projectOn('p-other', 'local'))
      useHomeWorkspaceStateMock.mockReturnValue({ wsId: null, error: false })

      render(<HomeRoute />)

      expect(screen.getByText(/unavailable/i)).toBeInTheDocument()
    })

    // The last-known home workspace is shown read-only; the host being down is no reason to hide it.
    it('keeps showing a cached home workspace while its host is down', () => {
      pairRemoteHost('h1')
      getOrCreateHostRuntime('h1').getState().setStatus('down')
      loadProjects(projectOn('p-other', 'local'))
      useHomeWorkspaceStateMock.mockReturnValue({ wsId: 'ws-cached', error: false, stale: true })

      const { container } = render(<HomeRoute />)

      expect(container).toBeEmptyDOMElement()
    })
  })
})
