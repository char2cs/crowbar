import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'

const { activeEffectsSpy, providersSpy } = vi.hoisted(() => ({
  activeEffectsSpy: vi.fn((_wsId: string, _active: boolean) => {}),
  providersSpy: vi.fn((_wsId: string) => {}),
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-effects', () => ({
  useWorkspaceEffects: (wsId: string, active: boolean) => activeEffectsSpy(wsId, active),
}))

vi.mock('@/features/agent/hooks/use-chat-providers', () => ({
  useChatProviders: (wsId: string) => providersSpy(wsId),
}))

import { WorkspaceView } from '@/features/workspace/components/workspace-view'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

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
  providersSpy.mockClear()
})

afterEach(() => {
  cleanup()
})

describe('WorkspaceView keep-alive lifecycle', () => {
  it('keeps the data feeds mounted while hidden and tells them whether the workspace is active', async () => {
    const { setActive } = await renderView(true)
    expect(activeEffectsSpy).toHaveBeenLastCalledWith('ws-a', true)

    await setActive(false)
    expect(activeEffectsSpy).toHaveBeenLastCalledWith('ws-a', false)

    await setActive(true)
    expect(activeEffectsSpy).toHaveBeenLastCalledWith('ws-a', true)
  })

  it('loads the chat providers for as long as the workspace is MOUNTED, active or not', async () => {
    const { setActive } = await renderView(true)
    expect(providersSpy).toHaveBeenCalledWith('ws-a')

    providersSpy.mockClear()
    await setActive(false)

    expect(providersSpy).toHaveBeenCalledWith('ws-a')
  })
})
