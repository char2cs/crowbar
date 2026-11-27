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
  useParams: () => ({ projectId: 'p1' }),
}))

const { useHomeWorkspaceStateMock } = vi.hoisted(() => ({
  useHomeWorkspaceStateMock: vi.fn(),
}))

vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  useHomeWorkspaceState: useHomeWorkspaceStateMock,
}))

import { ProjectHomeNotice } from '@/features/workspace/components/project-home-notice'

describe('ProjectHomeNotice', () => {
  it.each([
    ['while the home workspace id is still resolving', { wsId: null, error: false }],
    ['once resolved, when WorkspaceHost paints the content', { wsId: 'ws-home-1', error: false }],
  ])('renders nothing %s', (_name, state) => {
    useHomeWorkspaceStateMock.mockReturnValue(state)
    const { container } = render(<ProjectHomeNotice />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders fallback on error, saying what to do', () => {
    useHomeWorkspaceStateMock.mockReturnValue({ wsId: null, error: true })
    render(<ProjectHomeNotice />)
    expect(screen.getByText(/unavailable/i)).toBeInTheDocument()
    expect(screen.getByText(/connect to this project's host/i)).toBeInTheDocument()
  })

  describe('on an install that paired a remote host that is down', () => {
    afterEach(() => {
      resetHostWorld()
      destroyHostRuntime('h1')
    })

    it.each([
      ['says Project Home is unavailable while the only host that may hold it is down', null, true],
      ['keeps showing a cached home workspace', 'ws-cached', false],
    ])('%s', (_name, wsId, noticeShown) => {
      pairRemoteHost('h1')
      getOrCreateHostRuntime('h1').getState().setStatus('down')
      loadProjects(projectOn('p-other', 'local'))
      useHomeWorkspaceStateMock.mockReturnValue({ wsId, error: false })

      const { container } = render(<ProjectHomeNotice />)

      expect(container.textContent?.includes('unavailable') ?? false).toBe(noticeShown)
    })
  })
})
