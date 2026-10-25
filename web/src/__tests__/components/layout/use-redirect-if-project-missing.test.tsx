import { act, renderHook } from '@testing-library/react'
import type { NavigateFn } from '@tanstack/react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useRedirectIfProjectMissing } from '@/components/layout/use-redirect-if-project-missing'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import type { HostStatus } from '@/features/hosts/types'
import { loading, success } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import {
  loadProjects,
  markHostsRead,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const navigate = vi.fn()

function openRoute(projectId: string) {
  return renderHook(() => useRedirectIfProjectMissing(projectId, navigate as unknown as NavigateFn))
}

const setStatus = (hostId: string, status: HostStatus) =>
  getOrCreateHostRuntime(hostId).getState().setStatus(status)

beforeEach(() => {
  navigate.mockReset()
  resetHostWorld()
})
afterEach(() => {
  resetHostWorld()
  destroyHostRuntime('h1')
  destroyHostRuntime('h2')
})

describe('useRedirectIfProjectMissing where no remote host was ever paired', () => {
  it('leaves at the first loaded list that lacks the project, not while it loads', () => {
    openRoute('p1')
    act(() => useProjectDataStore.setState({ data: loading() }))
    expect(navigate).not.toHaveBeenCalled()

    act(() => loadProjects(projectOn('p2', 'local')))

    expect(navigate).toHaveBeenCalledExactlyOnceWith({ to: '/' })
  })

  it('leaves exactly once when the project is deleted while open', () => {
    loadProjects(projectOn('p1', 'local'), projectOn('p2', 'local'))
    openRoute('p1')

    act(() => loadProjects(projectOn('p2', 'local')))

    expect(navigate).toHaveBeenCalledExactlyOnceWith({ to: '/' })
  })
})

describe('useRedirectIfProjectMissing on an install that paired a remote host', () => {
  // The cache was written before p-new was created on h1: its route must survive the boot.
  it('keeps a route to a project no cached row holds while its host is unknown, up and read', () => {
    pairRemoteHost('h1')
    useProjectDataStore.setState({ data: loading(success([projectOn('p-local', 'local')])) })
    openRoute('p-new')

    act(() => loadProjects(projectOn('p-local', 'local')))
    act(() => setStatus('h1', 'up'))
    act(() => {
      loadProjects(projectOn('p-local', 'local'), projectOn('p-new', 'h1'))
      markHostsRead('h1')
    })

    expect(navigate).not.toHaveBeenCalled()
  })

  it('leaves at once when a local project is deleted while open, though a paired host is down', () => {
    pairRemoteHost('h1')
    setStatus('h1', 'down')
    loadProjects(projectOn('p1', 'local'), projectOn('p2', 'local'))
    openRoute('p1')

    act(() => loadProjects(projectOn('p2', 'local')))

    expect(navigate).toHaveBeenCalledExactlyOnceWith({ to: '/' })
  })

  it('leaves a revoked host’s project at once, and a stale route to it once the other host is read', () => {
    pairRemoteHost('h1')
    pairRemoteHost('h2')
    setStatus('h2', 'down')
    loadProjects(projectOn('p-local', 'local'), projectOn('p-h1', 'h1'))
    const live = openRoute('p-h1')

    act(() => {
      useHostConfigStore.getState().removeHost('h1')
      loadProjects(projectOn('p-local', 'local'))
    })
    expect(navigate).toHaveBeenCalledExactlyOnceWith({ to: '/' })

    live.unmount()
    navigate.mockReset()
    openRoute('p-h1')
    expect(navigate).not.toHaveBeenCalled()
    act(() => markHostsRead('h2'))
    expect(navigate).toHaveBeenCalledExactlyOnceWith({ to: '/' })
  })
})
