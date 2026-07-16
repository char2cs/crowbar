import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'

// Focused WorkspaceView lifecycle tests for keep-alive semantics:
//  - hydrate runs once per mount, never again on a warm re-activation;
//  - a warm (hidden→active) transition reconciles open buffers against disk —
//    the workspace's file watcher is gated off while hidden, and agents keep
//    editing files in hidden worktrees, so without this the editors come back
//    stale (the destroy-on-switch behaviour re-hydrated and reconciled);
//  - the cold path does NOT double-reconcile (hydrateWorkspace already does).
const { hydrateSpy, reconcileSpy, activeEffectsSpy } = vi.hoisted(() => ({
  hydrateSpy: vi.fn(async (_wsId: string) => ({ layout: null, editorStates: [] })),
  reconcileSpy: vi.fn(async (_wsId: string) => {}),
  activeEffectsSpy: vi.fn((_wsId: string) => {}),
}))

vi.mock('@/lib/persistence/hydrate', () => ({
  hydrateWorkspace: (wsId: string) => hydrateSpy(wsId),
  reconcileWorkspaceBuffersWithDisk: (wsId: string) => reconcileSpy(wsId),
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: (wsId: string) => ({ __fakeStore: wsId }),
  setActiveWorkspaceId: vi.fn(),
}))

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

  it('warm re-activation: reconciles open buffers against disk, without re-hydrating', async () => {
    const { setActive } = await renderView(true)
    await setActive(false) // hide (another workspace became active)
    expect(reconcileSpy).not.toHaveBeenCalled()

    await setActive(true) // warm return

    expect(reconcileSpy).toHaveBeenCalledTimes(1)
    expect(reconcileSpy).toHaveBeenCalledWith('ws-a')
    expect(hydrateSpy).toHaveBeenCalledTimes(1) // still only the cold hydrate
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
