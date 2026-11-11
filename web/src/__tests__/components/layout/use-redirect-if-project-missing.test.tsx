import { act, renderHook } from '@testing-library/react'
import type { NavigateFn } from '@tanstack/react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useRedirectIfProjectMissing } from '@/components/layout/use-redirect-if-project-missing'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const navigate = vi.fn()

function openRoute(projectId: string) {
  return renderHook(() => useRedirectIfProjectMissing(projectId, navigate as unknown as NavigateFn))
}

beforeEach(() => {
  navigate.mockReset()
  resetHostWorld()
})

describe('useRedirectIfProjectMissing where no remote host was ever paired', () => {
  it('leaves at the first synced list that lacks the project, not before it syncs', () => {
    openRoute('p1')
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
  // The projects scope is the local daemon's: a project in no row may be on the paired host.
  it('keeps a route to a project the synced list does not hold', () => {
    pairRemoteHost('h1')
    openRoute('p-remote')

    act(() => loadProjects(projectOn('p-local', 'local')))

    expect(navigate).not.toHaveBeenCalled()
  })
})
