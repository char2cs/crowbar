import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'

// Focused WorkspaceView lifecycle tests for keep-alive semantics: a warm
// (hidden -> active) flip does no work of its own (disk reconciliation belongs
// to the files feed, see use-workspace-file-tree).
const { activeEffectsSpy, chatScopeSpy } = vi.hoisted(() => ({
  activeEffectsSpy: vi.fn((_wsId: string, _active: boolean) => {}),
  chatScopeSpy: vi.fn((_wsId: string) => {}),
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-effects', () => ({
  useWorkspaceEffects: (wsId: string, active: boolean) => activeEffectsSpy(wsId, active),
}))

vi.mock('@/features/agent/hooks/use-chat-scope', () => ({
  useChatScope: (wsId: string) => chatScopeSpy(wsId),
}))

import { WorkspaceView } from '@/features/workspace/components/workspace-view'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

// The host hands the view its store, and the active view's keyboard hooks read it.
const store = getOrCreateWorkspaceStore('ws-a')

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
  activeEffectsSpy.mockClear()
  chatScopeSpy.mockClear()
})

afterEach(() => {
  cleanup()
})

describe('WorkspaceView keep-alive lifecycle', () => {
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

  // Recents, background adoption and a pane on this workspace's chat all need
  // its chat scope held while the workspace is hidden.
  it('holds the chat scope for as long as the workspace is MOUNTED, active or not', async () => {
    const { setActive } = await renderView(true)
    expect(chatScopeSpy).toHaveBeenCalledWith('ws-a')

    chatScopeSpy.mockClear()
    await setActive(false)

    expect(chatScopeSpy).toHaveBeenCalledWith('ws-a')
  })
})
