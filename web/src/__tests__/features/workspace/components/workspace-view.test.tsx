import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'

// Focused WorkspaceView lifecycle tests for keep-alive semantics: hydrate runs
// once per mount and a warm (hidden -> active) flip does no work of its own
// (disk reconciliation belongs to the files feed, see use-workspace-file-tree).
const { hydrateSpy, reconcileSpy, activeEffectsSpy, agentChatsStreamSpy } = vi.hoisted(() => ({
  hydrateSpy: vi.fn(async (_wsId: string) => ({ layout: null, editorStates: [] })),
  reconcileSpy: vi.fn(async (_wsId: string) => {}),
  activeEffectsSpy: vi.fn((_wsId: string, _active: boolean) => {}),
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
  useWorkspaceEffects: (wsId: string, active: boolean) => activeEffectsSpy(wsId, active),
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

  it('warm re-activation: only flips the feeds, with no re-hydrate and no disk re-read', async () => {
    const { setActive } = await renderView(true)
    await setActive(false)
    await setActive(true)

    expect(hydrateSpy).toHaveBeenCalledTimes(1) // still only the cold hydrate
    expect(reconcileSpy).not.toHaveBeenCalled()
  })

  // The tree/git feeds stay mounted while hidden so a switch back is a
  // selection, not a refetch; `active` only tells them whether to do a first load.
  it('keeps the data feeds mounted while hidden and tells them whether the workspace is active', async () => {
    const { setActive } = await renderView(true)
    expect(activeEffectsSpy).toHaveBeenLastCalledWith('ws-a', true)

    await setActive(false)
    expect(activeEffectsSpy).toHaveBeenLastCalledWith('ws-a', false)

    await setActive(true)
    expect(activeEffectsSpy).toHaveBeenLastCalledWith('ws-a', true)
  })

  // The agent feed seeds this workspace's providers/chats and feeds `working`;
  // the Recents band, "anything running has a row" and a pane still holding this
  // workspace's chat all need it live while the workspace is hidden.
  it('runs the agent chats stream for as long as the workspace is MOUNTED, active or not', async () => {
    const { setActive } = await renderView(true)
    expect(agentChatsStreamSpy).toHaveBeenCalledWith('ws-a')

    agentChatsStreamSpy.mockClear()
    await setActive(false)

    expect(agentChatsStreamSpy).toHaveBeenCalledWith('ws-a')
  })
})
