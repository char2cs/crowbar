import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'

// Focused WorkspaceView lifecycle tests for keep-alive semantics:
//  - hydrate runs once per mount, never again on a warm re-activation;
//  - a warm (hidden→active) transition reconciles open buffers against disk —
//    the workspace's file watcher is gated off while hidden, and agents keep
//    editing files in hidden worktrees, so without this the editors come back
//    stale (the destroy-on-switch behaviour re-hydrated and reconciled);
//  - the cold path does NOT double-reconcile (hydrateWorkspace already does).
const { hydrateSpy, reconcileSpy, activeEffectsSpy, agentChatsStreamSpy } = vi.hoisted(() => ({
  hydrateSpy: vi.fn(async (_wsId: string) => ({ layout: null, editorStates: [] })),
  reconcileSpy: vi.fn(async (_wsId: string) => {}),
  activeEffectsSpy: vi.fn((_wsId: string) => {}),
  agentChatsStreamSpy: vi.fn((_wsId: string) => {}),
}))

vi.mock('@/lib/persistence/hydrate', () => ({
  hydrateWorkspace: (wsId: string) => hydrateSpy(wsId),
  reconcileWorkspaceBuffersWithDisk: (wsId: string) => reconcileSpy(wsId),
}))

// The host hands the view its store; a stable fake is all the view needs.
const store = { __fakeStore: 'ws-a' } as unknown as WorkspaceStore

vi.mock('@/features/workspace/components/workspace-layout-root', () => ({
  WorkspaceLayoutRoot: () => <div data-testid="layout-root" />,
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-effects', () => ({
  useWorkspaceEffects: (wsId: string) => activeEffectsSpy(wsId),
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-agent-chats-stream', () => ({
  useWorkspaceAgentChatsStream: (wsId: string) => agentChatsStreamSpy(wsId),
}))

vi.mock('@/features/keymaps/hooks/use-save-keyboard', () => ({ useSaveKeyboard: () => {} }))
vi.mock('@/features/panes/hooks/use-pane-keyboard', () => ({ usePaneKeyboard: () => {} }))
vi.mock('@/features/keymaps/hooks/use-sidebar-tab-keyboard', () => ({
  useSidebarTabKeyboard: () => {},
}))

import { WorkspaceView } from '@/features/workspace/components/workspace-view'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

async function renderView(active: boolean) {
  let result!: ReturnType<typeof render>
  await act(async () => {
    result = render(<WorkspaceView wsId="ws-a" store={store} active={active} />)
  })
  const setActive = async (next: boolean) => {
    await act(async () => {
      result.rerender(<WorkspaceView wsId="ws-a" store={store} active={next} />)
    })
  }
  return { setActive }
}

beforeEach(() => {
  hydrateSpy.mockClear()
  reconcileSpy.mockClear()
  activeEffectsSpy.mockClear()
  agentChatsStreamSpy.mockClear()
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

  // The agent feed is NOT one of the active-only watchers. It seeds this
  // workspace's providers/chats and feeds `working`, and three surfaces need
  // that live while the workspace is hidden: the project-wide Recents band
  // (recents-for-project.ts aggregates every retained workspace), spec Law 9
  // ("anything running has a row"), and a window-level pane still holding this
  // workspace's chat. Its only previous mount point was the Chats panel Task 8
  // deleted, so nothing fed any of it at all.
  it('runs the agent chats stream for as long as the workspace is MOUNTED, active or not', async () => {
    const { setActive } = await renderView(true)
    expect(agentChatsStreamSpy).toHaveBeenCalledWith('ws-a')

    agentChatsStreamSpy.mockClear()
    activeEffectsSpy.mockClear()
    await setActive(false)

    // Hidden, but still mounted: the hook is still being called every render,
    // unlike the active-only watchers above.
    expect(agentChatsStreamSpy).toHaveBeenCalledWith('ws-a')
    expect(activeEffectsSpy).not.toHaveBeenCalled()
  })
})
