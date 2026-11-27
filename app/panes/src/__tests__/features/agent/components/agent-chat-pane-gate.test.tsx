import { act, cleanup, render, screen } from '@testing-library/react'
import { createElement, useEffect } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { pendingChatSurfaces } from '@/features/panes/stores/pending-chat-surfaces'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import { seedChatPaneRecord } from '@/__tests__/__fixtures__/view-state'
import { hostIdForWorkspace } from '@/lib/host-resolve'
import { entityStore } from '@/core/entities/entity-store'
import { getDB } from '@/lib/persistence/idb'
import { IDBFactory } from 'fake-indexeddb'
import {
  hydrateHostPlacements,
  startHostPlacementCache,
} from '@/features/hosts/lib/host-placements'
import { chatEntity, putEntities, workspaceDTO } from '@/__tests__/__fixtures__/sidebar-entities'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const probe = vi.hoisted(() => ({ mounts: 0 }))
vi.mock('@/features/agent/chat/agent-chat-view', () => ({
  AgentChatView: ({ visible }: { visible: boolean }) => {
    useEffect(() => {
      probe.mounts++
    }, [])
    return createElement(
      'div',
      { 'data-testid': 'chat-view', 'data-visible': String(visible) },
      createElement('textarea', { 'aria-label': 'composer' }),
    )
  },
}))

import { AgentChatPaneGate } from '@/features/agent/components/agent-chat-pane-gate'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'

const WAITING = 'Opening this chat…'

let fetchMock: ReturnType<typeof vi.fn>
const requested = () => fetchMock.mock.calls.map(([url]) => String(url))

function mount(wsId = 'w1', chatId = 'c1') {
  const store = createWorkspaceStore(wsId)
  seedChatPaneRecord(windowPaneStore, 'pane-1', chatId, null)
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(AgentChatPaneGate, {
        chatId,
        runnerId: '',
        wsId,
        paneId: 'pane-1',
        isActivePane: true,
        isVisible: true,
      }),
    ),
  )
}

beforeEach(() => {
  probe.mounts = 0
  resetHostWorld()
  resetWindowPaneStoreForTests()
  pendingChatSurfaces.setState({ pending: new Set() })
  useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
  recordWorkspaceScope({ projectId: 'p-local', repoId: 'r1', wsId: 'w1', owningChatId: 'o1' })
  fetchMock = vi.fn(async () => new Response(JSON.stringify({ success: true, data: [] })))
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  cleanup()
  resetHostWorld()
  destroyHostRuntime('host-1')
  vi.unstubAllGlobals()
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
})

describe('AgentChatPaneGate on a draft thread', () => {
  it('draws the blank composer and asks the daemon for no chat', async () => {
    await act(async () => loadProjects(projectOn('p-local', 'local')))

    mount('w1', 'draft-1')

    expect(
      await screen.findByRole('textbox', { name: /message the agent|describe the change/i }),
    ).toBeInTheDocument()
    expect(probe.mounts).toBe(0)
    expect(requested().filter((url) => /\/chats\/(?!providers)/.test(url))).toEqual([])
  })
})

describe('AgentChatPaneGate on an install that has paired a remote host', () => {
  beforeEach(() => pairRemoteHost('host-1'))

  it('says what it waits for, asks nothing and latches nothing, until the project list names the host', () => {
    mount()

    expect(screen.getByText(WAITING)).toBeInTheDocument()
    expect(screen.queryByTestId('chat-view')).toBeNull()
    expect(requested()).toEqual([])
    expect(probe.mounts).toBe(0)
  })

  it('opens the local chat once the list lands, with no retry needed', async () => {
    mount()

    await act(async () => loadProjects(projectOn('p-local', 'local')))

    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
    expect(probe.mounts).toBe(1)
    expect(screen.queryByText(WAITING)).toBeNull()
  })

  it('reads a remote chat from its own host once the list lands', async () => {
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
    mount('w-remote')
    expect(requested()).toEqual([])

    await act(async () => loadProjects(projectOn('p-remote', 'host-1')))

    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
    expect(requested().every((url) => url.startsWith('crowbar://host-1/'))).toBe(true)
  })

  it('keeps opening a chat no row holds until its host is read, then mounts', async () => {
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
    mount('w-remote')

    await act(async () => loadProjects(projectOn('p-local', 'local')))
    await act(async () => getOrCreateHostRuntime('host-1').getState().setStatus('up'))
    expect(screen.getByText(WAITING)).toBeInTheDocument()

    await act(async () =>
      loadProjects(projectOn('p-local', 'local'), projectOn('p-remote', 'host-1')),
    )
    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
  })

  it('says the host is not available while the host it may be on needs repair', async () => {
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
    mount('w-remote')

    await act(async () => loadProjects(projectOn('p-local', 'local')))
    await act(async () => getOrCreateHostRuntime('host-1').getState().setStatus('needs-repair'))

    expect(screen.getByText('Chat unavailable')).toBeInTheDocument()
    expect(screen.getByText(/not available right now/i)).toBeInTheDocument()
    expect(requested()).toEqual([])
  })

  it('does not blame the project when it is listed on a host that is not configured', async () => {
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
    mount('w-remote')

    await act(async () => loadProjects(projectOn('p-remote', 'host-gone')))

    expect(screen.getByText('Chat unavailable')).toBeInTheDocument()
    expect(screen.queryByText(/close this tab/i)).toBeNull()
    expect(screen.getByText(/not available right now/i)).toBeInTheDocument()
  })

  it('treats a workspace whose scope is not recorded yet as waiting, then mounts when it is', async () => {
    useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
    loadProjects(projectOn('p-local', 'local'))
    mount()
    expect(screen.getByText(WAITING)).toBeInTheDocument()
    expect(screen.queryByText('Chat unavailable')).toBeNull()

    await act(async () =>
      recordWorkspaceScope({ projectId: 'p-local', repoId: 'r1', wsId: 'w1', owningChatId: 'o1' }),
    )

    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
  })

  it('ends the pane’s wait for its surface even while it is only waiting', () => {
    pendingChatSurfaces.getState().begin('pane-1')

    mount()

    expect(pendingChatSurfaces.getState().pending.has('pane-1')).toBe(false)
  })

  it('unmounts the whole pane when its host stops resolving and mounts it afresh on return', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
    const keydown = vi.spyOn(window, 'addEventListener')
    const keyup = vi.spyOn(window, 'removeEventListener')
    const liveKeydown = () =>
      keydown.mock.calls.filter(([type]) => type === 'keydown').length -
      keyup.mock.calls.filter(([type]) => type === 'keydown').length
    mount('w-remote')
    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
    const whileMounted = liveKeydown()
    expect(whileMounted).toBeGreaterThan(0)

    act(() => useHostConfigStore.getState().removeHost('host-1'))

    expect(screen.queryByTestId('chat-view')).toBeNull()
    expect(liveKeydown()).toBeLessThan(whileMounted)

    act(() => {
      pairRemoteHost('host-1')
      loadProjects(projectOn('p-remote', 'host-1'))
    })

    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
    expect(probe.mounts).toBe(2)
    expect(liveKeydown()).toBe(whileMounted)
  })
})

describe('AgentChatPaneGate where no remote host was ever paired', () => {
  it('mounts at once, whatever the project list holds', async () => {
    mount()

    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
    expect(screen.queryByText(WAITING)).toBeNull()
  })
})

describe('AgentChatPaneGate before any host has listed', () => {
  async function lastRunPlaced(hostId: string, wsId = 'w-remote') {
    const stop = startHostPlacementCache()
    putEntities('workspace', [workspaceDTO({ id: wsId })], hostId)
    putEntities('chat', [chatEntity({ id: 'c1' })], hostId)
    await vi.waitFor(async () =>
      expect((await (await getDB()).getAll('host-placements')).length).toBe(2),
    )
    stop()
    resetHostWorld()
    pairRemoteHost('host-1')
    await hydrateHostPlacements()
  }

  beforeEach(async () => {
    globalThis.indexedDB = new IDBFactory()
    await hydrateHostPlacements()
    pairRemoteHost('host-1')
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
  })

  it('mounts the chat on the host the last run placed it on, with no list', async () => {
    await lastRunPlaced('host-1')

    mount('w-remote')

    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
    expect(screen.queryByText(WAITING)).toBeNull()
  })

  it('shows the recalled chat at once: its placement vouches for it until the list lands', async () => {
    await lastRunPlaced('host-1')

    mount('w-remote')

    expect(await screen.findByTestId('chat-view')).toHaveAttribute('data-visible', 'true')
  })

  it('does not route a call that creates data by the recalled host', async () => {
    await lastRunPlaced('host-1')

    expect(() => hostIdForWorkspace('w-remote')).toThrow()
  })

  it('stays mounted when the list lands and agrees', async () => {
    await lastRunPlaced('host-1')
    mount('w-remote')
    const view = await screen.findByTestId('chat-view')

    await act(async () => loadProjects(projectOn('p-remote', 'host-1')))
    act(() => putEntities('workspace', [workspaceDTO({ id: 'w-remote' })], 'host-1'))

    expect(screen.getByTestId('chat-view')).toBe(view)
    expect(probe.mounts).toBe(1)
  })

  it('leaves for the empty state when the list lands without the workspace', async () => {
    await lastRunPlaced('host-1')
    mount('w-remote')
    await screen.findByTestId('chat-view')

    await act(async () => loadProjects(projectOn('p-local', 'local'), projectOn('p-x', 'host-1')))

    expect(screen.queryByTestId('chat-view')).toBeNull()
    expect(screen.getByText('Chat unavailable')).toBeInTheDocument()
  })

  it('ignores a placement on a host that was removed', async () => {
    await lastRunPlaced('host-gone')

    mount('w-remote')

    expect(screen.getByText(WAITING)).toBeInTheDocument()
    expect(screen.queryByTestId('chat-view')).toBeNull()
  })

  it('opens the local chat from its placement while a paired host has not listed', async () => {
    await lastRunPlaced('local', 'w1')

    mount('w1')

    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
    expect(entityStore.getState().loaded).toEqual({})
  })

  it('waits as before when nothing could be read back', async () => {
    vi.stubGlobal('indexedDB', undefined)
    await hydrateHostPlacements()

    mount('w-remote')

    expect(screen.getByText(WAITING)).toBeInTheDocument()
  })
})
