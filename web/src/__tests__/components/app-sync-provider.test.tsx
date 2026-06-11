import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { success } from '@/lib/loadable'
import { AppSyncProvider } from '@/components/app-sync-provider'

describe('AppSyncProvider', () => {
  it('fetches workspaces + projects on mount and unsubscribes on unmount', () => {
    const wsUnsub = vi.fn()
    const pUnsub = vi.fn()
    vi.spyOn(useWorkspaceListStore.getState(), 'fetch').mockResolvedValue(undefined)
    vi.spyOn(useProjectDataStore.getState(), 'fetch').mockResolvedValue(undefined)
    const wsSync = vi.spyOn(useWorkspaceListStore.getState(), 'startSync').mockReturnValue(wsUnsub)
    const pSync = vi.spyOn(useProjectDataStore.getState(), 'startSync').mockReturnValue(pUnsub)

    const { unmount } = render(
      <AppSyncProvider>
        <div>child</div>
      </AppSyncProvider>,
    )
    expect(wsSync).toHaveBeenCalledOnce()
    expect(pSync).toHaveBeenCalledOnce()
    unmount()
    expect(wsUnsub).toHaveBeenCalledOnce()
    expect(pUnsub).toHaveBeenCalledOnce()
  })

  // Regression for BUG-014: a /v0/ws/workspaces push refetches the workspace
  // list, but the sidebar tree renders from useSidebarStore.repos, which was
  // only seeded once at boot. The provider must propagate fresh workspace-list
  // data into the sidebar so cross-client creates appear without a reload.
  describe('workspace-list → sidebar propagation (BUG-014)', () => {
    const REPO: Repo = {
      id: 'r1',
      name: 'repo',
      avatarLabel: 'R',
      avatarColor: 'bg-primary',
      workspaces: [{ id: 'ws-new', branch: 'feature/x', age: 'now' }],
    }

    beforeEach(() => {
      useSidebarStore.setState({ repos: [] })
      vi.spyOn(useWorkspaceListStore.getState(), 'fetch').mockResolvedValue(undefined)
      vi.spyOn(useProjectDataStore.getState(), 'fetch').mockResolvedValue(undefined)
      vi.spyOn(useWorkspaceListStore.getState(), 'startSync').mockReturnValue(() => {})
      vi.spyOn(useProjectDataStore.getState(), 'startSync').mockReturnValue(() => {})
    })

    it('merges refetched workspaces into the sidebar while mounted', async () => {
      render(
        <AppSyncProvider>
          <div />
        </AppSyncProvider>,
      )

      // Simulate the refetch triggered by a /v0/ws/workspaces push completing.
      useWorkspaceListStore.setState({ data: success([REPO]) })

      await waitFor(() => {
        const repos = useSidebarStore.getState().repos
        expect(repos.map((r) => r.id)).toContain('r1')
        expect(repos.find((r) => r.id === 'r1')?.workspaces.map((w) => w.id)).toContain('ws-new')
      })
    })

    it('stops propagating after unmount', () => {
      const { unmount } = render(
        <AppSyncProvider>
          <div />
        </AppSyncProvider>,
      )
      unmount()
      useWorkspaceListStore.setState({ data: success([REPO]) })
      expect(useSidebarStore.getState().repos).toEqual([])
    })
  })
})
