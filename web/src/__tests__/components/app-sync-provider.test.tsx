import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { AppSyncProvider } from '@/components/app-sync-provider'

describe('AppSyncProvider', () => {
  it('fetches workspaces + projects on mount and unsubscribes on unmount', () => {
    const wsUnsub = vi.fn(); const pUnsub = vi.fn()
    vi.spyOn(useWorkspaceListStore.getState(), 'fetch').mockResolvedValue(undefined)
    vi.spyOn(useProjectDataStore.getState(), 'fetch').mockResolvedValue(undefined)
    const wsSync = vi.spyOn(useWorkspaceListStore.getState(), 'startSync').mockReturnValue(wsUnsub)
    const pSync = vi.spyOn(useProjectDataStore.getState(), 'startSync').mockReturnValue(pUnsub)

    const { unmount } = render(<AppSyncProvider><div>child</div></AppSyncProvider>)
    expect(wsSync).toHaveBeenCalledOnce()
    expect(pSync).toHaveBeenCalledOnce()
    unmount()
    expect(wsUnsub).toHaveBeenCalledOnce()
    expect(pUnsub).toHaveBeenCalledOnce()
  })
})
