import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'

// Hoisted spies so the vi.mock factories (hoisted above imports) can capture them.
// `events` records unmount/destroy interleaving: components living over the store
// (Monaco panes, terminal slots) must be UNMOUNTED before the store is destroyed.
const { hydrateSpy, destroySpy, events } = vi.hoisted(() => {
  const events = [] as string[]
  return {
    events,
    hydrateSpy: vi.fn<(wsId: string) => void>(),
    destroySpy: vi.fn<(wsId: string) => void>((wsId) => {
      events.push(`destroy:${wsId}`)
    }),
  }
})

// Light WorkspaceView stub: mirrors the real hydrate-once-per-mount contract
// (one hydrate per mount, never on a warm re-activation) without pulling the
// Monaco/terminal subtree into the test.
vi.mock('@/features/workspace/components/workspace-view', async () => {
  const React = await import('react')
  return {
    WorkspaceView: ({ wsId, active }: { wsId: string; active: boolean }) => {
      React.useEffect(() => {
        hydrateSpy(wsId)
        return () => {
          events.push(`unmount:${wsId}`)
        }
      }, [wsId])
      return React.createElement('div', {
        'data-testid': `wsview-${wsId}`,
        'data-active': String(active),
      })
    },
  }
})

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  destroyWorkspaceStore: (wsId: string) => destroySpy(wsId),
}))

import { WorkspaceHost } from '@/features/workspace/components/workspace-host'
import { useSettingsStore } from '@/features/settings/store'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'

const MIN = 60_000

function setKeepAlive(minutes: number) {
  act(() => {
    useSettingsStore.setState((s) => ({
      settings: { ...s.settings, workspaceKeepAliveMinutes: minutes },
    }))
  })
}

function seedSidebar(ids: string[], opts: { defaultWorkspaceId?: string } = {}) {
  const repo: Repo = {
    id: 'repo1',
    name: 'repo1',
    avatarLabel: 'R',
    avatarColor: '#000',
    workspaces: ids.map((id) => ({ id, branch: id, age: 'now' })),
    ...(opts.defaultWorkspaceId ? { defaultWorkspaceId: opts.defaultWorkspaceId } : {}),
  }
  act(() => {
    useSidebarStore.setState({ repos: [repo] })
  })
}

function slot(wsId: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-workspace-slot="${wsId}"]`)
}

function hydrateCount(wsId: string): number {
  return hydrateSpy.mock.calls.filter((c) => c[0] === wsId).length
}

beforeEach(() => {
  vi.useFakeTimers()
  hydrateSpy.mockClear()
  destroySpy.mockClear()
  events.length = 0
  useSidebarStore.setState(getInitialState())
  setKeepAlive(10)
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('WorkspaceHost', () => {
  it('mounts the active workspace and hydrates it once', () => {
    render(<WorkspaceHost activeWsId="a" />)
    expect(slot('a')).not.toBeNull()
    expect(slot('a')!.style.display).toBe('contents')
    expect(hydrateCount('a')).toBe(1)
  })

  it('keeps the previous workspace mounted and does not re-hydrate on a warm return (A→B→A)', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    expect(hydrateCount('a')).toBe(1)

    // Switch A → B: A stays mounted but hidden (display:none — see
    // workspace-slot-style.ts) + inert, B is active.
    rerender(<WorkspaceHost activeWsId="b" />)
    expect(slot('a')).not.toBeNull()
    expect(slot('a')!.style.display).toBe('none')
    expect(slot('a')!.hasAttribute('inert')).toBe(true)
    expect(slot('b')!.style.display).toBe('contents')
    expect(hydrateCount('b')).toBe(1)
    expect(destroySpy).not.toHaveBeenCalled()

    // Switch back B → A: A was never destroyed and is not re-hydrated.
    rerender(<WorkspaceHost activeWsId="a" />)
    expect(slot('a')!.style.display).toBe('contents')
    expect(slot('b')!.style.display).toBe('none')
    expect(hydrateCount('a')).toBe(1) // still only the original cold hydrate
    expect(destroySpy).not.toHaveBeenCalled()
  })

  it('retains workspaces across a project-home transit (activeWsId=null) and returns warm', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    expect(hydrateCount('a')).toBe(1)

    // Navigate to project home: no workspace is active. The host stays mounted
    // (it is the IDE-session-long content host), so retention must NOT be wiped.
    rerender(<WorkspaceHost activeWsId={null} />)
    expect(slot('a')).not.toBeNull()
    expect(slot('a')!.style.display).toBe('none')
    expect(slot('a')!.hasAttribute('inert')).toBe(true)
    expect(destroySpy).not.toHaveBeenCalled()

    // Return home → A: warm (no re-hydrate), and A was never destroyed.
    rerender(<WorkspaceHost activeWsId="a" />)
    expect(slot('a')!.style.display).toBe('contents')
    expect(hydrateCount('a')).toBe(1)
    expect(destroySpy).not.toHaveBeenCalled()
  })

  it('renders nothing (no crash) when mounted directly with no active workspace (home landing)', () => {
    const { container } = render(<WorkspaceHost activeWsId={null} />)
    expect(container.querySelector('[data-workspace-slot]')).toBeNull()
    expect(destroySpy).not.toHaveBeenCalled()
  })

  it('on home, the most-recent workspace survives but older retained ones still age out', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    act(() => {
      vi.advanceTimersByTime(1000)
    })
    rerender(<WorkspaceHost activeWsId="b" />) // a hidden+retained, b active (most recent)

    // Go to project home — both stay retained across the transit.
    rerender(<WorkspaceHost activeWsId={null} />)
    expect(slot('a')).not.toBeNull()
    expect(slot('b')).not.toBeNull()

    // Sit on home past the window: A ages out, but B (most-recently used) is
    // still protected even though nothing is active.
    act(() => {
      vi.advanceTimersByTime(10 * MIN + 1000)
    })
    expect(destroySpy).toHaveBeenCalledWith('a')
    expect(slot('a')).toBeNull()
    expect(destroySpy).not.toHaveBeenCalledWith('b')
    expect(slot('b')).not.toBeNull()
  })

  it('evicts (destroys) a hidden workspace after its keep-alive window elapses', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    rerender(<WorkspaceHost activeWsId="b" />)
    expect(slot('a')).not.toBeNull()
    expect(destroySpy).not.toHaveBeenCalled()

    // Advance past the 10-minute window; the armed timer evicts A.
    act(() => {
      vi.advanceTimersByTime(10 * MIN + 1000)
    })

    expect(destroySpy).toHaveBeenCalledWith('a')
    expect(slot('a')).toBeNull()
    expect(slot('b')!.style.display).toBe('contents')
  })

  it('does not evict the active workspace even after it has sat idle past the window', () => {
    render(<WorkspaceHost activeWsId="a" />)
    act(() => {
      vi.advanceTimersByTime(60 * MIN)
    })
    // No timer was ever armed for the sole active workspace; it stays mounted.
    expect(destroySpy).not.toHaveBeenCalledWith('a')
    expect(slot('a')).not.toBeNull()
  })

  it('caps retained workspaces at 6, evicting the oldest', () => {
    const ids = ['w0', 'w1', 'w2', 'w3', 'w4', 'w5', 'w6']
    const { rerender } = render(<WorkspaceHost activeWsId={ids[0]} />)
    for (let i = 1; i < ids.length; i++) {
      // Advance a little between activations so the recency order is strict.
      act(() => {
        vi.advanceTimersByTime(1000)
      })
      rerender(<WorkspaceHost activeWsId={ids[i]} />)
    }

    // The oldest (w0) is pushed out by the cap despite being within the window.
    expect(destroySpy).toHaveBeenCalledWith('w0')
    expect(slot('w0')).toBeNull()
    for (const id of ids.slice(1)) {
      expect(slot(id)).not.toBeNull()
    }
  })

  it('keepAliveMinutes=0 destroys the previous workspace on switch (A→B destroys A)', () => {
    setKeepAlive(0)
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    rerender(<WorkspaceHost activeWsId="b" />)

    expect(destroySpy).toHaveBeenCalledWith('a')
    expect(slot('a')).toBeNull()
    expect(slot('b')!.style.display).toBe('contents')
  })

  it('destroys a retained workspace once it no longer exists (closed / deleted)', () => {
    seedSidebar(['a', 'b'])
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    rerender(<WorkspaceHost activeWsId="b" />)
    expect(slot('a')).not.toBeNull()
    expect(destroySpy).not.toHaveBeenCalled()

    // "Close" workspace A: it disappears from the sidebar's workspace set.
    seedSidebar(['b'])

    expect(destroySpy).toHaveBeenCalledWith('a')
    expect(slot('a')).toBeNull()
  })

  it('protects a homeWsIds entry from the existence-prune — home is a project-level concept, absent from the sidebar entirely', () => {
    // Sidebar only knows about repo workspace "b" — the home id is never a
    // tree row, so without homeWsIds it would look "closed" (same shape as
    // the "destroys a retained workspace once it no longer exists" case
    // above) the instant it goes hidden.
    seedSidebar(['b'])
    const { rerender } = render(<WorkspaceHost activeWsId="home-ws" homeWsIds={['home-ws']} />)
    rerender(<WorkspaceHost activeWsId="b" homeWsIds={['home-ws']} />)

    expect(slot('home-ws')).not.toBeNull()
    expect(slot('home-ws')!.style.display).toBe('none')
    expect(destroySpy).not.toHaveBeenCalled()

    // And a warm return to it needs no re-hydration.
    rerender(<WorkspaceHost activeWsId="home-ws" homeWsIds={['home-ws']} />)
    expect(slot('home-ws')!.style.display).toBe('contents')
    expect(hydrateCount('home-ws')).toBe(1)
  })

  it('a homeWsIds entry is still evicted by the ordinary keep-alive TTL (homeWsIds only exempts it from the existence-prune)', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="home-ws" homeWsIds={['home-ws']} />)
    rerender(<WorkspaceHost activeWsId="b" homeWsIds={['home-ws']} />)
    expect(slot('home-ws')).not.toBeNull()

    act(() => {
      vi.advanceTimersByTime(10 * MIN + 1000)
    })

    expect(destroySpy).toHaveBeenCalledWith('home-ws')
    expect(slot('home-ws')).toBeNull()
  })

  it('retains the DEFAULT (main-worktree) workspace when hidden — it lives in repo.defaultWorkspaceId, not the workspaces array', () => {
    // The default workspace is not a tree row: it exists only as
    // repo.defaultWorkspaceId. Pruning against repo.workspaces alone would
    // destroy it the moment it goes hidden while any child exists.
    seedSidebar(['child1'], { defaultWorkspaceId: 'def-ws' })
    const { rerender } = render(<WorkspaceHost activeWsId="def-ws" />)
    rerender(<WorkspaceHost activeWsId="child1" />)

    expect(destroySpy).not.toHaveBeenCalled()
    expect(slot('def-ws')).not.toBeNull()
    expect(slot('def-ws')!.style.display).toBe('none')

    // And a warm return to it needs no re-hydration.
    rerender(<WorkspaceHost activeWsId="def-ws" />)
    expect(hydrateCount('def-ws')).toBe(1)
  })

  it('unmounts an evicted workspace BEFORE destroying its store (never a live subtree over a dead store)', () => {
    setKeepAlive(0)
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    events.length = 0
    rerender(<WorkspaceHost activeWsId="b" />)

    // Monaco panes / terminal slots in A's subtree must be torn down before the
    // store they read from is destroyed.
    expect(events).toContain('unmount:a')
    expect(events).toContain('destroy:a')
    expect(events.indexOf('unmount:a')).toBeLessThan(events.indexOf('destroy:a'))
  })

  it('unmount-before-destroy also holds on the TIMER eviction path', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    rerender(<WorkspaceHost activeWsId="b" />)
    events.length = 0

    act(() => {
      vi.advanceTimersByTime(10 * MIN + 1000)
    })

    expect(events.indexOf('unmount:a')).toBeGreaterThanOrEqual(0)
    expect(events.indexOf('unmount:a')).toBeLessThan(events.indexOf('destroy:a'))
  })
})
