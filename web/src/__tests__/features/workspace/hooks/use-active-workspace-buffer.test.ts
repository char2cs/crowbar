import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'

const routerState = { wsId: 'ws1' }
vi.mock('@tanstack/react-router', () => ({
  useRouterState: ({ select }: { select: (s: unknown) => unknown }) =>
    select({ location: { pathname: `/workspaces/${routerState.wsId}` } }),
}))

const fakeStore = {
  state: {
    buffers: [{ id: 'b1', type: 'editor' }],
    paneActions: {
      getActivePane: () => ({ activeBufferId: 'b1' as string | null }),
    },
  },
  listeners: new Set<() => void>(),
  getState() {
    return this.state
  },
  subscribe(fn: () => void) {
    this.listeners.add(fn)
    return () => this.listeners.delete(fn)
  },
}
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => fakeStore,
}))

import { useActiveWorkspaceBuffer } from '@/features/workspace/hooks/use-active-workspace-buffer'

beforeEach(() => {
  routerState.wsId = 'ws1'
  fakeStore.state = {
    buffers: [{ id: 'b1', type: 'editor' }],
    paneActions: { getActivePane: () => ({ activeBufferId: 'b1' }) },
  }
})

describe('useActiveWorkspaceBuffer', () => {
  it('returns the active buffer of the active workspace', () => {
    const { result } = renderHook(() => useActiveWorkspaceBuffer())
    expect(result.current).toEqual({ id: 'b1', type: 'editor' })
  })

  it('returns null when there is no workspace in the route', () => {
    routerState.wsId = ''
    const { result } = renderHook(() => useActiveWorkspaceBuffer())
    expect(result.current).toBeNull()
  })

  it('returns null when the active pane has no active buffer', () => {
    fakeStore.state = {
      buffers: [{ id: 'b1', type: 'editor' }],
      paneActions: { getActivePane: () => ({ activeBufferId: null }) },
    }
    const { result } = renderHook(() => useActiveWorkspaceBuffer())
    expect(result.current).toBeNull()
  })

  it('re-renders with the new buffer when the store notifies a change', () => {
    const { result } = renderHook(() => useActiveWorkspaceBuffer())
    expect(result.current).toEqual({ id: 'b1', type: 'editor' })

    act(() => {
      fakeStore.state = {
        buffers: [{ id: 'b2', type: 'webViewer' }],
        paneActions: {
          getActivePane: () => ({ activeBufferId: 'b2' as string | null }),
        },
      }
      fakeStore.listeners.forEach((fn) => fn())
    })

    expect(result.current).toEqual({ id: 'b2', type: 'webViewer' })
  })

  it('keeps a stable reference when an unrelated notification fires', () => {
    const { result } = renderHook(() => useActiveWorkspaceBuffer())
    const first = result.current

    act(() => {
      // Same state object/contents → snapshot returns the same buffer reference.
      fakeStore.listeners.forEach((fn) => fn())
    })

    expect(result.current).toBe(first)
  })
})
