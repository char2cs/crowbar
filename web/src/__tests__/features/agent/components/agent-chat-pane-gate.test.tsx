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
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import { seedChatPaneRecord } from '@/__tests__/__fixtures__/view-state'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

// The chat view is where a pane's chat is first asked for (its feed). A probe stands in
// for it, so the gate is judged by whether that view mounts and what reaches the
// network. Its textarea stands in for the view's composer.
const probe = vi.hoisted(() => ({ mounts: 0 }))
vi.mock('@/features/agent/chat/agent-chat-view', () => ({
  AgentChatView: () => {
    useEffect(() => {
      probe.mounts++
    }, [])
    return createElement(
      'div',
      { 'data-testid': 'chat-view' },
      createElement('textarea', { 'aria-label': 'composer' }),
    )
  },
}))

import { AgentChatPaneGate } from '@/features/agent/components/agent-chat-pane-gate'

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
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p-local', repoId: 'r1', wsId: 'w1', owningChatId: 'o1' })
  fetchMock = vi.fn(async () => new Response(JSON.stringify({ success: true, data: [] })))
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  // Unmount before the world resets: resetting it would let a gate that was still
  // mounted mount its pane and read the network after the stub is gone.
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
    // The provider list rides along for the picker; no chat is read or created.
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
    // Whatever the pane reads (its host's provider list) goes to that host, never the local one.
    expect(requested().every((url) => url.startsWith('crowbar://host-1/'))).toBe(true)
  })

  // A pane restored at boot whose project is in no cached row: the first local answer does
  // not say it is gone while the host it may be on has not been read.
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
    __resetWorkspaceScopesForTest()
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

  // The body owns every pane hook and window listener: they go with it and come back
  // with it, rather than acting on a blank pane or watching a container that was replaced.
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

    act(() => pairRemoteHost('host-1'))

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
