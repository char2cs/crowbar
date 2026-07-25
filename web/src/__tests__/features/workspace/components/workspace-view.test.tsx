import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'

// Focused WorkspaceView lifecycle tests for keep-alive semantics:
//  - hydrate runs once per mount, never again on a warm re-activation;
//  - a warm (hidden→active) transition reconciles open buffers against disk —
//    the workspace's file watcher is gated off while hidden, and agents keep
//    editing files in hidden worktrees, so without this the editors come back
//    stale (the destroy-on-switch behaviour re-hydrated and reconciled);
//  - the cold path does NOT double-reconcile (hydrateWorkspace already does).
const { hydrateSpy, reconcileSpy, activeEffectsSpy, openNewTabSpy } = vi.hoisted(() => ({
  hydrateSpy: vi.fn(async (_wsId: string) => ({ layout: null, editorStates: [] })),
  reconcileSpy: vi.fn(async (_wsId: string) => {}),
  activeEffectsSpy: vi.fn((_wsId: string) => {}),
  openNewTabSpy: vi.fn(),
}))

vi.mock('@/lib/persistence/hydrate', () => ({
  hydrateWorkspace: (wsId: string) => hydrateSpy(wsId),
  reconcileWorkspaceBuffersWithDisk: (wsId: string) => reconcileSpy(wsId),
}))

// getOrCreateWorkspaceStore memoizes by wsId in real life (a registry singleton
// map) — the fake must too, or every render would hand useOpenOnNewTab a
// freshly-identitied store and refire it regardless of whether `hydrated`
// actually changed.
//
// `panes` mirrors production's real shape (see pane-slice's initial state):
// ROOT_PANE_ID and BOTTOM_PANE_ID both always exist, each independently empty
// or not — useOpenOnNewTab (I3) seeds a New Tab PER empty pane, not once per
// workspace.
vi.mock('@/features/workspace/stores/workspace-store-registry', () => {
  const fakeStores = new Map<string, unknown>()
  return {
    getOrCreateWorkspaceStore: (wsId: string) => {
      if (!fakeStores.has(wsId)) {
        fakeStores.set(wsId, {
          __fakeStore: wsId,
          getState: () => ({
            buffers: [],
            panes: {
              [ROOT_PANE_ID]: { id: ROOT_PANE_ID, bufferIds: [] },
              [BOTTOM_PANE_ID]: { id: BOTTOM_PANE_ID, bufferIds: [] },
            },
            bufferActions: { openNewTab: openNewTabSpy },
          }),
        })
      }
      return fakeStores.get(wsId)
    },
    setActiveWorkspaceId: vi.fn(),
  }
})

vi.mock('@/features/workspace/stores/workspace-store-ref', () => ({
  setActiveWorkspaceStoreRef: vi.fn(),
}))

vi.mock('@/features/workspace/components/workspace-layout-root', () => ({
  WorkspaceLayoutRoot: () => <div data-testid="layout-root" />,
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-effects', () => ({
  useWorkspaceEffects: (wsId: string) => activeEffectsSpy(wsId),
}))

vi.mock('@/features/keymaps/hooks/use-save-keyboard', () => ({ useSaveKeyboard: () => {} }))
vi.mock('@/features/panes/hooks/use-pane-keyboard', () => ({ usePaneKeyboard: () => {} }))
vi.mock('@/features/keymaps/hooks/use-sidebar-tab-keyboard', () => ({
  useSidebarTabKeyboard: () => {},
}))

import { WorkspaceView } from '@/features/workspace/components/workspace-view'

async function renderView(active: boolean) {
  let result!: ReturnType<typeof render>
  await act(async () => {
    result = render(<WorkspaceView wsId="ws-a" active={active} />)
  })
  const setActive = async (next: boolean) => {
    await act(async () => {
      result.rerender(<WorkspaceView wsId="ws-a" active={next} />)
    })
  }
  return { setActive }
}

beforeEach(() => {
  hydrateSpy.mockClear()
  reconcileSpy.mockClear()
  activeEffectsSpy.mockClear()
  openNewTabSpy.mockClear()
})

afterEach(() => {
  cleanup()
})

describe('WorkspaceView keep-alive lifecycle', () => {
  it('cold mount: hydrates once and does NOT disk-reconcile again (hydrate already does)', async () => {
    await renderView(true)

    expect(hydrateSpy).toHaveBeenCalledTimes(1)
    expect(hydrateSpy).toHaveBeenCalledWith('ws-a')
    expect(reconcileSpy).not.toHaveBeenCalled()
  })

  it('opens a New Tab in every empty RENDERED pane once hydration lands on a workspace with nothing restored', async () => {
    await renderView(true)

    // ROOT_PANE_ID restored empty and is rendered, so it gets seeded.
    expect(openNewTabSpy).toHaveBeenCalledWith(ROOT_PANE_ID)
    // BOTTOM_PANE_ID is never rendered (nothing draws `bottomLayout`), so a New
    // Tab there would be an invisible, auto-eviction-PROTECTED buffer spending
    // one of MAX_OPEN_TABS forever — see use-open-on-new-tab.
    expect(openNewTabSpy).not.toHaveBeenCalledWith(BOTTOM_PANE_ID)
    expect(openNewTabSpy).toHaveBeenCalledTimes(1)
  })

  it('warm re-activation: reconciles open buffers against disk, without re-hydrating', async () => {
    const { setActive } = await renderView(true)
    await setActive(false) // hide (another workspace became active)
    expect(reconcileSpy).not.toHaveBeenCalled()

    await setActive(true) // warm return

    expect(reconcileSpy).toHaveBeenCalledTimes(1)
    expect(reconcileSpy).toHaveBeenCalledWith('ws-a')
    expect(hydrateSpy).toHaveBeenCalledTimes(1) // still only the cold hydrate
    expect(openNewTabSpy).toHaveBeenCalledTimes(1) // still only the cold open, not re-fired by the hide/return
  })

  it('runs the workspace watchers only while active', async () => {
    const { setActive } = await renderView(true)
    expect(activeEffectsSpy).toHaveBeenCalledWith('ws-a')

    activeEffectsSpy.mockClear()
    await setActive(false)
    // Hidden: the active-effects subtree is unmounted; nothing re-invokes it.
    expect(activeEffectsSpy).not.toHaveBeenCalled()

    await setActive(true)
    expect(activeEffectsSpy).toHaveBeenCalledWith('ws-a')
  })
})
