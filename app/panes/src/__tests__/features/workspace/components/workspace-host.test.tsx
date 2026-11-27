import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'

const { hydrateSpy, destroySpy, events, registry, rendering, layoutMounts, ambientSeen } =
  vi.hoisted(() => {
    const events = [] as string[]
    const registry = new Map<string, { wsId: string }>()
    return {
      events,
      registry,
      layoutMounts: { current: 0 },
      ambientSeen: [] as Array<string | undefined>,
      rendering: { current: false, mintedWhileRendering: 0 },
      hydrateSpy: vi.fn<(wsId: string) => void>(),
      destroySpy: vi.fn<(wsId: string) => void>((wsId) => {
        events.push(`destroy:${wsId}`)
        registry.delete(wsId)
      }),
    }
  })

vi.mock('@/features/workspace/components/workspace-view', async () => {
  const React = await import('react')
  return {
    WorkspaceView: ({ wsId, active }: { wsId: string; active: boolean }) => {
      rendering.current = true
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
  setActiveWorkspaceId: () => {},
}))

vi.mock('@/features/workspace/components/workspace-layout-root', async () => {
  const React = await import('react')
  const ctx = await import('@/features/workspace/stores/workspace-context')
  return {
    WorkspaceLayoutRoot: () => {
      const { WorkspaceStoreContext } = ctx
      ambientSeen.push((React.useContext(WorkspaceStoreContext) as { wsId?: string } | null)?.wsId)
      React.useEffect(() => {
        layoutMounts.current++
      }, [])
      return null
    },
  }
})

import { WorkspaceHost } from '@/features/workspace/components/workspace-host'
import { requestWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'
import type { Repo } from '@/core/entities/repo-types'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import * as perf from '@/lib/perf/instrumentation'

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
    seedRepos([repo])
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
  rendering.mintedWhileRendering = 0
  layoutMounts.current = 0
  ambientSeen.length = 0
  resetEntities()
})

afterEach(() => {
  cleanup()
})

function mountedSlots(): string[] {
  return [...document.querySelectorAll<HTMLElement>('[data-workspace-slot]')]
    .map((el) => el.dataset.workspaceSlot!)
    .sort()
}

describe('WorkspaceHost — the pane tree across a switch', () => {
  it('keeps the ambient store handed to the pane tree the same when the active workspace flips', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a', 'b']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a', 'b']} />)
    rerender(<WorkspaceHost activeWsId="a" viewWsIds={['a', 'b']} />)

    expect(new Set(ambientSeen.filter(Boolean)).size).toBe(1)
  })
})

describe('WorkspaceHost — warm-switch span', () => {
  function hostSwitchSpans(spy: { mock: { calls: unknown[][] } }): number {
    return spy.mock.calls.filter((c) => c[0] === 'workspace.switch').length
  }

  it('does not span a cold switch to a workspace that was not retained', () => {
    const spy = vi.spyOn(perf, 'markStart')
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    spy.mockClear()
    rerender(<WorkspaceHost activeWsId="b" />)
    expect(hostSwitchSpans(spy)).toBe(0)
    spy.mockRestore()
  })

  it('spans a warm switch back to a retained workspace', () => {
    const spy = vi.spyOn(perf, 'markStart')
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a', 'b']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a', 'b']} />)
    spy.mockClear()
    rerender(<WorkspaceHost activeWsId="a" viewWsIds={['a', 'b']} />)
    expect(hostSwitchSpans(spy)).toBe(1)
    spy.mockRestore()
  })
})

describe('WorkspaceHost — sole owner of the registry', () => {
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

  it('mounts every workspace a pane names, past any count, and keeps the registry equal to the slots', () => {
    const many = Array.from({ length: 10 }, (_, i) => `p${i}`)
    render(<WorkspaceHost activeWsId="a" paneWsIds={many} />)
    expect(mountedSlots()).toEqual(['a', ...many].sort())
    expect(registry.size).toBe(mountedSlots().length)
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

    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a', 'b']} />)
    expect(slot('a')).not.toBeNull()
    expect(slot('a')!.style.display).toBe('none')
    expect(slot('a')!.hasAttribute('inert')).toBe(true)
    expect(slot('b')!.style.display).toBe('contents')
    expect(hydrateCount('b')).toBe(1)
    expect(destroySpy).not.toHaveBeenCalled()

    rerender(<WorkspaceHost activeWsId="a" viewWsIds={['a', 'b']} />)
    expect(slot('a')!.style.display).toBe('contents')
    expect(slot('b')!.style.display).toBe('none')
    expect(hydrateCount('a')).toBe(1)
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

    rerender(<WorkspaceHost activeWsId="b" viewWsIds={[]} />)

    expect(destroySpy).toHaveBeenCalledWith('a')
    expect(slot('a')).toBeNull()
  })

  it('retains workspaces across a project-home transit (activeWsId=null) and returns warm', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    expect(hydrateCount('a')).toBe(1)

    rerender(<WorkspaceHost activeWsId={null} viewWsIds={['a']} />)
    expect(slot('a')).not.toBeNull()
    expect(slot('a')!.style.display).toBe('none')
    expect(slot('a')!.hasAttribute('inert')).toBe(true)
    expect(destroySpy).not.toHaveBeenCalled()

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
    rerender(<WorkspaceHost activeWsId="b" />)
    expect(destroySpy).toHaveBeenCalledWith('a')

    rerender(<WorkspaceHost activeWsId={null} />)
    expect(destroySpy).toHaveBeenCalledWith('b')
    expect(slot('b')).toBeNull()
  })

  it('never unmounts the window pane tree while a cold workspace is being activated', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a', 'b']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a', 'b']} />)
    rerender(<WorkspaceHost activeWsId="c" viewWsIds={['a', 'b', 'c']} />)
    expect(layoutMounts.current).toBe(1)
  })

  it('keeps EVERY workspace that has a view mounted, however many — switching between Recents rows never cold-mounts', () => {
    const ids = Array.from({ length: 9 }, (_, i) => `w${i}`)
    const { rerender } = render(<WorkspaceHost activeWsId={ids[0]} viewWsIds={ids} />)
    for (const id of ids) rerender(<WorkspaceHost activeWsId={id} viewWsIds={ids} />)
    for (const id of ids) rerender(<WorkspaceHost activeWsId={id} viewWsIds={ids} />)

    expect(mountedSlots()).toEqual([...ids].sort())
    expect(destroySpy).not.toHaveBeenCalled()
    for (const id of ids) expect(hydrateCount(id)).toBe(1)
  })

  it('destroys a retained workspace once it no longer exists (closed / deleted), even while it has a view chat', () => {
    seedSidebar(['a', 'b'])
    const { rerender } = render(<WorkspaceHost activeWsId="a" viewWsIds={['a']} />)
    rerender(<WorkspaceHost activeWsId="b" viewWsIds={['a']} />)
    expect(slot('a')).not.toBeNull()
    expect(destroySpy).not.toHaveBeenCalled()

    seedSidebar(['b'])

    expect(destroySpy).toHaveBeenCalledWith('a')
    expect(slot('a')).toBeNull()
  })

  it('protects a homeWsIds entry from the existence-prune — home is a project-level concept, absent from the sidebar entirely', () => {
    seedSidebar(['b'])
    const { rerender } = render(
      <WorkspaceHost activeWsId="home-ws" homeWsIds={['home-ws']} viewWsIds={['home-ws']} />,
    )
    rerender(<WorkspaceHost activeWsId="b" homeWsIds={['home-ws']} viewWsIds={['home-ws']} />)

    expect(slot('home-ws')).not.toBeNull()
    expect(slot('home-ws')!.style.display).toBe('none')
    expect(destroySpy).not.toHaveBeenCalled()

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

    rerender(<WorkspaceHost activeWsId="a" paneWsIds={['a']} viewWsIds={[]} />)

    expect(destroySpy).toHaveBeenCalledWith('b')
    expect(slot('b')).toBeNull()
  })

  it('retains the DEFAULT (main-worktree) workspace when hidden — it lives in repo.defaultWorkspaceId, not the workspaces array', () => {
    seedSidebar(['child1'], { defaultWorkspaceId: 'def-ws' })
    const { rerender } = render(<WorkspaceHost activeWsId="def-ws" viewWsIds={['def-ws']} />)
    rerender(<WorkspaceHost activeWsId="child1" viewWsIds={['def-ws']} />)

    expect(destroySpy).not.toHaveBeenCalled()
    expect(slot('def-ws')).not.toBeNull()
    expect(slot('def-ws')!.style.display).toBe('none')

    rerender(<WorkspaceHost activeWsId="def-ws" viewWsIds={['def-ws']} />)
    expect(hydrateCount('def-ws')).toBe(1)
  })

  it('unmounts an evicted workspace BEFORE destroying its store (never a live subtree over a dead store)', () => {
    const { rerender } = render(<WorkspaceHost activeWsId="a" />)
    events.length = 0
    rerender(<WorkspaceHost activeWsId="b" />)

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
