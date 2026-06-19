import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, waitFor } from '@testing-library/react'

// §7 startup is driven by GET seeds + subscribeEntityStream subscriptions and a
// one-time maybeWipeOnVersionChange() BEFORE seeding. We mock those seams and
// assert the provider wires them up and tears every subscription down on unmount.
const { wipe, subscribeEntityStream, fetchRepos, fetchWorkspaces } = vi.hoisted(() => ({
  wipe: vi.fn().mockResolvedValue(undefined),
  subscribeEntityStream: vi.fn(),
  fetchRepos: vi.fn(),
  fetchWorkspaces: vi.fn(),
}))

vi.mock('@/lib/persistence/idb', async () => {
  const actual = await vi.importActual<typeof import('@/lib/persistence/idb')>(
    '@/lib/persistence/idb',
  )
  return { ...actual, maybeWipeOnVersionChange: wipe }
})

vi.mock('@/lib/ws/entity-stream', () => ({
  subscribeEntityStream: (...args: unknown[]) => subscribeEntityStream(...args),
}))

vi.mock('@/lib/api', () => ({
  fetchRepos: (...args: unknown[]) => fetchRepos(...args),
  fetchWorkspaces: (...args: unknown[]) => fetchWorkspaces(...args),
  fetchProjects: vi.fn().mockResolvedValue([]),
}))

import { AppSyncProvider } from '@/components/app-sync-provider'
import { useProjectStore } from '@/lib/store/projects'
import { useProjectDataStore } from '@/lib/store/projects'

beforeEach(() => {
  vi.clearAllMocks()
  subscribeEntityStream.mockReturnValue(() => {})
  fetchRepos.mockResolvedValue([])
  fetchWorkspaces.mockResolvedValue([])
  useProjectStore.setState({ activeProjectId: 'p1' })
  vi.spyOn(useProjectDataStore.getState(), 'fetch').mockResolvedValue(undefined)
  vi.spyOn(useProjectDataStore.getState(), 'startSync').mockReturnValue(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('AppSyncProvider §7 startup', () => {
  it('wipes the cache on version change BEFORE seeding, then seeds projects', async () => {
    render(
      <AppSyncProvider>
        <div>child</div>
      </AppSyncProvider>,
    )
    await waitFor(() => expect(wipe).toHaveBeenCalledOnce())
    await waitFor(() => expect(useProjectDataStore.getState().fetch).toHaveBeenCalled())
  })

  it("subscribes the active project's repos entity stream", async () => {
    const repoStream = subscribeEntityStream.mock
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await waitFor(() => expect(repoStream.calls.length).toBeGreaterThan(0))
    const reposCall = repoStream.calls
      .map((c) => c[0] as { endpoint: string })
      .find((o) => o.endpoint === '/v0/projects/p1/repos')
    expect(reposCall).toBeDefined()
  })

  it('tears every subscription down on unmount', async () => {
    const unsub = vi.fn()
    subscribeEntityStream.mockReturnValue(unsub)
    const { unmount } = render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await waitFor(() => expect(subscribeEntityStream).toHaveBeenCalled())
    unmount()
    expect(unsub).toHaveBeenCalled()
  })
})
