import type { ReactNode } from 'react'
import { afterEach, describe, expect, it } from 'vitest'
import { act, cleanup, renderHook } from '@testing-library/react'
import { usePaneWorkspace } from '@/features/panes/hooks/use-pane-workspace'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'

function setup() {
  const a = getOrCreateWorkspaceStore('ws-a')
  const b = getOrCreateWorkspaceStore('ws-b')
  setActiveWorkspaceId('ws-a')
  // The ambient store never follows the active workspace.
  const wrapper = ({ children }: { children: ReactNode }) => (
    <WorkspaceStoreContext.Provider value={a}>{children}</WorkspaceStoreContext.Provider>
  )
  return { a, b, wrapper }
}

afterEach(() => {
  cleanup()
  setActiveWorkspaceId(null)
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
})

describe('usePaneWorkspace', () => {
  it('gives a chat pane its own workspace and never re-renders when the active workspace flips', () => {
    const { b, wrapper } = setup()
    let renders = 0
    const { result } = renderHook(
      () => {
        renders += 1
        return usePaneWorkspace({ chatId: 'chat-1', workspaceId: 'ws-b' })
      },
      { wrapper },
    )
    expect(result.current.wsId).toBe('ws-b')
    expect(result.current.chatStore).toBe(b)
    const before = renders

    act(() => setActiveWorkspaceId('ws-b'))
    act(() => setActiveWorkspaceId('ws-a'))

    expect(renders).toBe(before)
  })

  it('makes a chatless pane follow the active workspace', () => {
    const { a, b, wrapper } = setup()
    const { result } = renderHook(() => usePaneWorkspace({ chatId: null, workspaceId: null }), {
      wrapper,
    })
    expect(result.current.wsId).toBe('ws-a')
    expect(result.current.chatStore).toBe(a)

    act(() => setActiveWorkspaceId('ws-b'))

    expect(result.current.wsId).toBe('ws-b')
    expect(result.current.chatStore).toBe(b)
  })
})
