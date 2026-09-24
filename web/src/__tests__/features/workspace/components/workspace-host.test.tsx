import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'

// Hoisted spies so the vi.mock factories (hoisted above imports) can capture them.
// `events` records unmount/destroy interleaving: components living over the store
// (Monaco panes, terminal slots) must be UNMOUNTED before the store is destroyed.
const { hydrateSpy, destroySpy, events, registry, pinned, rendering } = vi.hoisted(() => {
  const events = [] as string[]
  const registry = new Map<string, { wsId: string }>()
  return {
    events,
    registry,
    /** Workspaces whose editor is still mounted into a pane (canEvict false). */
    pinned: new Set<string>(),
    /** Set while a WorkspaceView renders, to catch a mint in the render path. */
    rendering: { current: false, mintedWhileRendering: 0 },
    hydrateSpy: vi.fn<(wsId: string) => void>(),
    destroySpy: vi.fn<(wsId: string) => void>((wsId) => {
      events.push(`destroy:${wsId}`)
      registry.delete(wsId)
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
      rendering.current = true
      // Children's layout effects run before the host's: the render phase is over.
      React.useLayoutEffect(() => {
        rendering.current = false
      })
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
  getOrCreateWorkspaceStore: (wsId: string) => {
    if (rendering.current) rendering.mintedWhileRendering++
    if (!registry.has(wsId)) registry.set(wsId, { wsId })
    return registry.get(wsId)
  },
  getWorkspaceStore: (wsId: string) => registry.get(wsId),
  canEvictWorkspace: (wsId: string) => !pinned.has(wsId),
  setActiveWorkspaceId: () => {},
}))

// The pane tree itself is another suite's subject; this one is about retention.
vi.mock('@/features/workspace/components/workspace-layout-root', () => ({
  WorkspaceLayoutRoot: () => null,
}))

import { WorkspaceHost } from '@/features/workspace/components/workspace-host'
import { requestWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'

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
  hydrateSpy.mockClear()
  destroySpy.mockClear()
  events.length = 0
  registry.clear()
  pinned.clear()
  rendering.mintedWhileRendering = 0
  useSidebarStore.setState(getInitialState())
})

afterEach(() => {
  cleanup()
})

/** The mounted slots, as the DOM shows them. */
function mountedSlots(): string[] {
  return [...document.querySelectorAll<HTMLElement>('[data-workspace-slot]')]
    .map((el) => el.dataset.workspaceSlot!)
    .sort()
}

describe('WorkspaceHost — sole owner of the registry (C5, C6)', () => {
  it('registry keys are exactly the mounted slots, through switches and evictions', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a', 'b']} />)
    expect([...registry.keys()].sort()).toEqual(mountedSlots())
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a', 'b']} />)
    expect([...registry.keys()].sort()).toEqual(mountedSlots())
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['b']} />)
    expect([...registry.keys()].sort()).toEqual(['b'])
    expect(mountedSlots()).toEqual(['b'])
  })

  it('never mints a store while rendering', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" paneWsIds={['p1', 'p2']} />)
    rerender(<WorkspaceHost activeWsId="c" paneWsIds={['p1', 'p3']} />)
    expect(rendering.mintedWhileRendering).toBe(0)
  })

  it('holds the cap even when panes name more workspaces than it allows (no force-mount past the plan)', () => {
    const many = Array.from({ length: 10 }, (_, i) => `p${i}`)
    render(<WorkspaceHost activeWsId="a" paneWsIds={many} />)
    expect(mountedSlots().length).toBeLessThanOrEqual(6)
    expect(registry.size).toBe(mountedSlots().length)
    expect(mountedSlots()).toContain('a')
  })

  it('keeps a workspace whose editor is still mounted instead of destroying it — no zombie store', () => {
    pinned.add('a')
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    rerender(<WorkspaceHost activeWsId="b" />)
    // Not evictable: still mounted, still registered, never destroyed.
    expect(destroySpy).not.toHaveBeenCalledWith('a')
    expect(mountedSlots()).toEqual(['a', 'b'])
    expect([...registry.keys()].sort()).toEqual(['a', 'b'])

    // Once its editor lets go, the next reconcile evicts it for real.
    pinned.delete('a')
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={[]} paneWsIds={['b']} />)
    expect(destroySpy).toHaveBeenCalledWith('a')
    expect([...registry.keys()]).toEqual(['b'])
  })
})

describe('WorkspaceHost', () => {
  it('mounts the active workspace and hydrates it once', () => {
    render(<WorkspaceHost activeWsId="a" />)
    expect(slot('a')).not.toBeNull()
    expect(slot('a')!.style.display).toBe('contents')
    expect(hydrateCount('a')).toBe(1)
  })

  it('keeps the previous workspace mounted and does not re-hydrate on a warm return (A→B→A) while both still have a view', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a', 'b']} />)
    expect(hydrateCount('a')).toBe(1)

    // Switch A → B: A stays mounted but hidden (display:none — see
    // workspace-slot-style.ts) + inert, B is active. Both have a view chat.
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a', 'b']} />)
    expect(slot('a')).not.toBeNull()
    expect(slot('a')!.style.display).toBe('none')
    expect(slot('a')!.hasAttribute('inert')).toBe(true)
    expect(slot('b')!.style.display).toBe('contents')
    expect(hydrateCount('b')).toBe(1)
    expect(destroySpy).not.toHaveBeenCalled()

    // Switch back B → A: A was never destroyed and is not re-hydrated.
    rerender(<WorkspaceHost activeWsId="a" viewWsIds={['a', 'b']} />)
    expect(slot('a')!.style.display).toBe('contents')
    expect(slot('b')!.style.display).toBe('none')
    expect(hydrateCount('a')).toBe(1) // still only the original cold hydrate
    expect(destroySpy).not.toHaveBeenCalled()
  })

  it('evicts the previous workspace IMMEDIATELY on switch when it has no view chat (no grace period)', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    rerender(<WorkspaceHost activeWsId="b" />)

    expect(destroySpy).toHaveBeenCalledWith('a')
    expect(slot('a')).toBeNull()
    expect(slot('b')!.style.display).toBe('contents')
  })

  it('retains a non-active workspace only as long as viewWsIds still names it, evicting it the instant it drops out', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a']} />)
    expect(slot('a')).not.toBeNull()
    expect(destroySpy).not.toHaveBeenCalled()

    // The view closes (its last chat leaves Recents): viewWsIds drops 'a'.
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={[]} />)

    expect(destroySpy).toHaveBeenCalledWith('a')
    expect(slot('a')).toBeNull()
  })

  it('retains workspaces across a project-home transit (activeWsId=null) and returns warm', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    expect(hydrateCount('a')).toBe(1)

    // Navigate to project home: no workspace is active, but A still has a
    // view chat, so it survives the transit.
    rerender(<WorkspaceHost activeWsId={null} viewWsIds={['a']} />)
    expect(slot('a')).not.toBeNull()
    expect(slot('a')!.style.display).toBe('none')
    expect(slot('a')!.hasAttribute('inert')).toBe(true)
    expect(destroySpy).not.toHaveBeenCalled()

    // Return home → A: warm (no re-hydrate), and A was never destroyed.
    rerender(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    expect(slot('a')!.style.display).toBe('contents')
    expect(hydrateCount('a')).toBe(1)
    expect(destroySpy).not.toHaveBeenCalled()
  })

  it('renders nothing (no crash) when mounted directly with no active workspace (home landing)', () => {
    const { container } = render(<WorkspaceHost activeWsId={null} />)
    expect(container.querySelector('[data-workspace-slot]')).toBeNull()
    expect(destroySpy).not.toHaveBeenCalled()
  })

  it('on home with no active workspace and no view chat anywhere, nothing is retained', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    rerender(<WorkspaceHost activeWsId="b" />) // a evicted immediately (no view chat)
    expect(destroySpy).toHaveBeenCalledWith('a')

    // Go to project home with no active workspace at all: b has no view
    // chat either, so nothing survives the transit.
    rerender(<WorkspaceHost activeWsId={null} />)
    expect(destroySpy).toHaveBeenCalledWith('b')
    expect(slot('b')).toBeNull()
  })

  it('caps retained workspaces at 6, evicting the least-recently-active over the cap', () => {
    vi.useFakeTimers()
    try {
      const ids = ['w0', 'w1', 'w2', 'w3', 'w4', 'w5', 'w6']
      // Every id has a view chat (so all 7 are candidates), only the active one
      // changes across renders. Advance a little between activations so the
      // cap's LRU tie-break has a strict recency order to sort by.
      const { rerender } = render(<WorkspaceHost activeWsId={ids[0]} viewWsIds={ids} />)
      for (let i = 1; i < ids.length; i++) {
        act(() => {
          vi.advanceTimersByTime(1000)
        })
        rerender(<WorkspaceHost activeWsId={ids[i]} viewWsIds={ids} />)
      }

      // The least-recently-activated (w0) is pushed out by the cap despite
      // still having a view chat.
      expect(destroySpy).toHaveBeenCalledWith('w0')
      expect(slot('w0')).toBeNull()
      for (const id of ids.slice(1)) {
        expect(slot(id)).not.toBeNull()
      }
    } finally {
      vi.useRealTimers()
    }
  })

  it('destroys a retained workspace once it no longer exists (closed / deleted), even while it has a view chat', () => {
    seedSidebar(['a', 'b'])
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a']} />)
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
    // above) the instant it goes hidden. Give it a view chat too, or the new
    // retention rule (not the existence-prune) would evict it anyway.
    seedSidebar(['b'])
    const { rerender } = render(
      <WorkspaceHost activeWsId="home-ws" homeWsIds={['home-ws']} viewWsIds={['home-ws']} />,
    )
    rerender(<WorkspaceHost activeWsId="b" homeWsIds={['home-ws']} viewWsIds={['home-ws']} />)

    expect(slot('home-ws')).not.toBeNull()
    expect(slot('home-ws')!.style.display).toBe('none')
    expect(destroySpy).not.toHaveBeenCalled()

    // And a warm return to it needs no re-hydration.
    rerender(<WorkspaceHost activeWsId="home-ws" homeWsIds={['home-ws']} viewWsIds={['home-ws']} />)
    expect(slot('home-ws')!.style.display).toBe('contents')
    expect(hydrateCount('home-ws')).toBe(1)
  })

  it('a homeWsIds entry is still evicted once it has no view chat — homeWsIds only exempts it from the existence-prune', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="home-ws" homeWsIds={['home-ws']} />)
    rerender(<WorkspaceHost activeWsId="b" homeWsIds={['home-ws']} />)

    expect(destroySpy).toHaveBeenCalledWith('home-ws')
    expect(slot('home-ws')).toBeNull()
  })

  // Regression: a split can hold panes from workspaces this host never
  // otherwise mounts (never routed to, never clicked into) — without a real
  // store, PaneContainer/ChatHead fell back to whichever workspace happened
  // to be AMBIENT, rendering wrong/blank until the OTHER pane's own click
  // happened to flip which workspace was active. `paneWsIds` force-mounts
  // every one of them, the same blank-frame guard `activeWsId` already gets.
  it("force-mounts every paneWsIds entry, even one that's neither active nor previously retained", () => {
    render(<WorkspaceHost activeWsId="a" paneWsIds={['a', 'b']} />)

    expect(slot('a')).not.toBeNull()
    expect(slot('a')!.style.display).toBe('contents')
    expect(slot('b')).not.toBeNull()
    expect(slot('b')!.style.display).toBe('none')
  })

  it('keeps a paneWsIds entry retained across reconciles — a pane holding a chat is inherently "in a view"', () => {
    render(<WorkspaceHost activeWsId="a" paneWsIds={['a', 'b']} viewWsIds={['b']} />)
    expect(slot('b')).not.toBeNull()
    expect(slot('b')!.style.display).toBe('none')
    expect(destroySpy).not.toHaveBeenCalledWith('b')
  })

  it('evicts a paneWsIds entry once it stops being pane-referenced and has no view chat', () => {
    const { rerender } = render(
      <WorkspaceHost activeWsId="a" paneWsIds={['a', 'b']} viewWsIds={['b']} />,
    )
    expect(slot('b')).not.toBeNull()

    // The pane closes and its chat leaves Recents entirely.
    rerender(<WorkspaceHost activeWsId="a" paneWsIds={['a']} viewWsIds={[]} />)

    expect(destroySpy).toHaveBeenCalledWith('b')
    expect(slot('b')).toBeNull()
  })

  it('retains the DEFAULT (main-worktree) workspace when hidden — it lives in repo.defaultWorkspaceId, not the workspaces array', () => {
    // The default workspace is not a tree row: it exists only as
    // repo.defaultWorkspaceId. Pruning against repo.workspaces alone would
    // destroy it the moment it goes hidden while any child exists.
    seedSidebar(['child1'], { defaultWorkspaceId: 'def-ws' })
    const { rerender } = render(<WorkspaceHost activeWsId="def-ws" viewWsIds={['def-ws']} />)
    rerender(<WorkspaceHost activeWsId="child1" viewWsIds={['def-ws']} />)

    expect(destroySpy).not.toHaveBeenCalled()
    expect(slot('def-ws')).not.toBeNull()
    expect(slot('def-ws')!.style.display).toBe('none')

    // And a warm return to it needs no re-hydration.
    rerender(<WorkspaceHost activeWsId="def-ws" viewWsIds={['def-ws']} />)
    expect(hydrateCount('def-ws')).toBe(1)
  })

  it('unmounts an evicted workspace BEFORE destroying its store (never a live subtree over a dead store)', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    events.length = 0
    rerender(<WorkspaceHost activeWsId="b" />)

    // Monaco panes / terminal slots in A's subtree must be torn down before the
    // store they read from is destroyed.
    expect(events).toContain('unmount:a')
    expect(events).toContain('destroy:a')
    expect(events.indexOf('unmount:a')).toBeLessThan(events.indexOf('destroy:a'))
  })

  it('unmount-before-destroy also holds when a view chat drops out later', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a']} />)
    events.length = 0

    rerender(<WorkspaceHost activeWsId="b" viewWsIds={[]} />)

    expect(events.indexOf('unmount:a')).toBeGreaterThanOrEqual(0)
    expect(events.indexOf('unmount:a')).toBeLessThan(events.indexOf('destroy:a'))
  })
})

/**
 * Forced eviction — the close path asking for a workspace to go NOW
 * (workspace-eviction-request.ts). Independent of the ordinary retention
 * test: a workspace whose last VIEW the user just closed is exactly the case
 * the new rule already handles the instant `viewWsIds` catches up, but the
 * close path also has this synchronous, no-wait-for-props escape hatch.
 */
describe('WorkspaceHost — forced eviction', () => {
  it('drops a retained workspace immediately', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a']} />)
    expect(slot('a')).not.toBeNull()
    expect(destroySpy).not.toHaveBeenCalled()

    act(() => requestWorkspaceEviction('a'))

    expect(slot('a')).toBeNull()
    expect(destroySpy).toHaveBeenCalledWith('a')
  })

  it('still unmounts before destroying — the same rule the ordinary path keeps', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a']} />)
    events.length = 0

    act(() => requestWorkspaceEviction('a'))

    expect(events.indexOf('unmount:a')).toBeGreaterThanOrEqual(0)
    expect(events.indexOf('unmount:a')).toBeLessThan(events.indexOf('destroy:a'))
  })

  // The ACTIVE workspace is the route. Its `WorkspaceView` is mounted over the
  // store and would re-create it the instant it went away.
  it('refuses to evict the workspace currently on screen', () => {
    render(<WorkspaceHost activeWsId="a" />)

    act(() => requestWorkspaceEviction('a'))

    expect(slot('a')).not.toBeNull()
    expect(destroySpy).not.toHaveBeenCalled()
  })

  it('is a no-op for a workspace this host never mounted', () => {
    render(<WorkspaceHost activeWsId="a" />)

    act(() => requestWorkspaceEviction('never-seen'))

    expect(destroySpy).not.toHaveBeenCalled()
    expect(slot('a')).not.toBeNull()
  })

  it('stops listening once the host unmounts', () => {
    const { rerender, unmount } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a']} />)
    unmount()
    destroySpy.mockClear()

    act(() => requestWorkspaceEviction('a'))

    expect(destroySpy).not.toHaveBeenCalled()
  })
})
