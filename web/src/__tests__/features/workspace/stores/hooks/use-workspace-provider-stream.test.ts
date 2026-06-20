import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'

// The per-:wsId workspace WS subscription is load-bearing beyond data: the
// daemon starts the per-connection provider poll (PR status detection) ONLY when
// a client subscribes to the SINGLE-workspace stream. The repo-level workspace
// LIST stream resolves an empty scope and never starts it, so a workspace whose
// branch has an open PR stayed 'new' (plain branch glyph) forever. This hook is
// what opens that stream while a workspace is viewed; these tests lock that wiring.
const { subscribeEntityStream, fetchWorkspace, syncSidebarFromCache } = vi.hoisted(() => ({
  subscribeEntityStream: vi.fn(),
  fetchWorkspace: vi.fn(),
  syncSidebarFromCache: vi.fn(),
}))

vi.mock('@/lib/ws/entity-stream', () => ({
  subscribeEntityStream: (...args: unknown[]) => subscribeEntityStream(...args),
}))

vi.mock('@/lib/api', () => ({
  fetchWorkspace: (...args: unknown[]) => fetchWorkspace(...args),
}))

vi.mock('@/lib/store/sidebar-sync', () => ({
  syncSidebarFromCache: (...args: unknown[]) => syncSidebarFromCache(...args),
}))

import { useWorkspaceProviderStream } from '@/features/workspace/stores/hooks/use-workspace-provider-stream'

beforeEach(() => {
  vi.clearAllMocks()
  subscribeEntityStream.mockReturnValue(() => {})
  fetchWorkspace.mockResolvedValue({ id: 'w1' })
})

describe('useWorkspaceProviderStream', () => {
  it('subscribes to the per-:wsId workspace stream when the full scope is present', () => {
    renderHook(() => useWorkspaceProviderStream('p1', 'r1', 'w1'))

    expect(subscribeEntityStream).toHaveBeenCalledTimes(1)
    const opts = subscribeEntityStream.mock.calls[0][0] as {
      endpoint: string
      store: string
    }
    expect(opts.endpoint).toBe('/v0/projects/p1/repos/r1/workspaces/w1')
    expect(opts.store).toBe('crowbar_workspaces')
  })

  it('does NOT subscribe when any scope id is missing', () => {
    const { rerender } = renderHook<void, { p?: string; r?: string; w?: string }>(
      ({ p, r, w }) => useWorkspaceProviderStream(p, r, w),
      { initialProps: { p: 'p1', r: 'r1', w: undefined } },
    )
    expect(subscribeEntityStream).not.toHaveBeenCalled()

    rerender({ p: 'p1', r: undefined, w: 'w1' })
    expect(subscribeEntityStream).not.toHaveBeenCalled()

    rerender({ p: undefined, r: 'r1', w: 'w1' })
    expect(subscribeEntityStream).not.toHaveBeenCalled()
  })

  it('unsubscribes on unmount (mirrors the daemon refcount lifecycle)', () => {
    const unsub = vi.fn()
    subscribeEntityStream.mockReturnValue(unsub)

    const { unmount } = renderHook(() => useWorkspaceProviderStream('p1', 'r1', 'w1'))
    expect(subscribeEntityStream).toHaveBeenCalledTimes(1)

    unmount()
    expect(unsub).toHaveBeenCalledTimes(1)
  })

  it('tears down the old stream and opens a new one when the viewed workspace changes', () => {
    const unsubW1 = vi.fn()
    const unsubW2 = vi.fn()
    subscribeEntityStream.mockReturnValueOnce(unsubW1).mockReturnValueOnce(unsubW2)

    const { rerender } = renderHook(
      ({ w }: { w: string }) => useWorkspaceProviderStream('p1', 'r1', w),
      { initialProps: { w: 'w1' } },
    )
    expect(subscribeEntityStream.mock.calls[0][0]).toMatchObject({
      endpoint: '/v0/projects/p1/repos/r1/workspaces/w1',
    })

    rerender({ w: 'w2' })
    expect(unsubW1).toHaveBeenCalledTimes(1)
    expect(subscribeEntityStream.mock.calls[1][0]).toMatchObject({
      endpoint: '/v0/projects/p1/repos/r1/workspaces/w2',
    })
  })
})
