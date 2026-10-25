import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createElement, useEffect } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { chatLedgerStore } from '@/features/agent/stores/chat-ledger-store'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { failed } from '@/lib/loadable'
import type { Project } from '@/lib/types'
import { projectListTransport } from '@/lib/api/project-list-transport'
import { connectProjectList, useProjectDataStore } from '@/lib/store/projects'
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
  markHostsRead,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

// The chat view is where a pane's first read happens (the ledger's first page). A probe
// makes that read through the real ledger and API, so the gate is judged by what reaches
// the network, not by what it renders. Its textarea stands in for the view's composer.
const probe = vi.hoisted(() => ({ mounts: 0 }))
vi.mock('@/features/agent/chat/agent-chat-view', () => ({
  AgentChatView: ({ wsId, chatId }: { wsId: string; chatId: string }) => {
    useEffect(() => {
      probe.mounts++
      void chatLedgerStore.getState().loadInitial(wsId, chatId)
    }, [wsId, chatId])
    return createElement(
      'div',
      { 'data-testid': 'chat-view' },
      createElement('textarea', { 'aria-label': 'composer' }),
    )
  },
}))

import { AgentChatPaneGate } from '@/features/agent/components/agent-chat-pane-gate'

const PAGE = { success: true, data: { cursor: 0, oldestCursor: 0, hasMore: false, items: [] } }
const WAITING = 'Opening this chat…'

const listAnswer = (...projects: Project[]) =>
  new Response(JSON.stringify({ success: true, data: projects }), { status: 200 })
// What the desktop proxy answers for a remote host it could not dial.
const unreachable = () =>
  new Response(null, { status: 502, headers: { 'x-crowbar-proxy': 'host-unreachable' } })

let fetchMock: ReturnType<typeof vi.fn>
const requested = () => fetchMock.mock.calls.map(([url]) => String(url))
// The pane's other reads (its daemon's provider list) ride along; the chat's is the one judged.
const chatReads = () => requested().filter((url) => url.includes('/messages'))

function mount(wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  seedChatPaneRecord(windowPaneStore, 'pane-1', 'c1', null)
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(AgentChatPaneGate, {
        chatId: 'c1',
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
  connectProjectList(projectListTransport)
  resetHostWorld()
  resetWindowPaneStoreForTests()
  chatLedgerStore.setState({ ledgers: {} })
  pendingChatSurfaces.setState({ pending: new Set() })
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p-local', repoId: 'r1', wsId: 'w1', owningChatId: 'o1' })
  fetchMock = vi.fn(async () => new Response(JSON.stringify(PAGE), { status: 200 }))
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

describe('AgentChatPaneGate on an install that has paired a remote host', () => {
  beforeEach(() => pairRemoteHost('host-1'))

  it('says what it waits for, asks nothing and latches nothing, until the project list names the host', () => {
    mount()

    expect(screen.getByText(WAITING)).toBeInTheDocument()
    expect(screen.queryByTestId('chat-view')).toBeNull()
    expect(requested()).toEqual([])
    expect(chatLedgerStore.getState().ledgers.c1).toBeUndefined()
  })

  it('reads the local chat from the local daemon once the list lands, with no retry needed', async () => {
    mount()

    await act(async () => loadProjects(projectOn('p-local', 'local')))

    await waitFor(() => expect(chatLedgerStore.getState().ledgers.c1?.status).toBe('loaded'))
    expect(chatReads()).toEqual(['/v0/projects/p-local/repos/r1/chats/c1/messages?limit=100'])
    expect(screen.queryByText(WAITING)).toBeNull()
  })

  it('reads a remote chat from its own host once the list lands', async () => {
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
    mount('w-remote')
    expect(requested()).toEqual([])

    await act(async () => loadProjects(projectOn('p-remote', 'host-1')))

    // The pane's other reads (its host's provider list) ride along; the chat's is the one judged.
    await waitFor(() =>
      expect(requested().filter((url) => url.includes('/messages'))).toHaveLength(1),
    )
    expect(requested()).toContain(
      'crowbar://host-1/v0/projects/p-remote/repos/r1/chats/c1/messages?limit=100',
    )
    expect(requested().every((url) => url.startsWith('crowbar://host-1/'))).toBe(true)
  })

  it('keeps saying it is waiting when the list could not be read and nothing is held', async () => {
    mount()

    await act(async () => useProjectDataStore.setState({ data: failed(new Error('daemon down')) }))

    expect(screen.getByText(WAITING)).toBeInTheDocument()
    expect(requested()).toEqual([])
  })

  it('says the project is gone, and reads nothing, once the list and every host lack it', async () => {
    mount()

    await act(async () => loadProjects(projectOn('p-other', 'local')))
    expect(screen.getByText(WAITING)).toBeInTheDocument()
    await act(async () => markHostsRead('host-1'))

    expect(screen.getByText('Chat unavailable')).toBeInTheDocument()
    expect(screen.getByText(/no longer in your list/i)).toBeInTheDocument()
    expect(requested()).toEqual([])
    expect(chatLedgerStore.getState().ledgers.c1).toBeUndefined()
  })

  // A pane restored at boot whose project is in no cached row: the first local answer does
  // not say it is gone while the host it may be on has not been read.
  it('keeps opening a chat no row holds until its host is read, then mounts', async () => {
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
    mount('w-remote')

    await act(async () => loadProjects(projectOn('p-local', 'local')))
    await act(async () => getOrCreateHostRuntime('host-1').getState().setStatus('up'))
    expect(screen.getByText(WAITING)).toBeInTheDocument()

    await act(async () => {
      loadProjects(projectOn('p-local', 'local'), projectOn('p-remote', 'host-1'))
      markHostsRead('host-1')
    })
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
    expect(chatReads()).toEqual(['/v0/projects/p-local/repos/r1/chats/c1/messages?limit=100'])
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

  // A re-read (the local socket reconnecting, a wake from sleep) that cannot reach the
  // remote host must not take its rows away: the pane, and what is typed in it, stay.
  it.each([
    ['down', 'down'],
    ['up, but its read fails', 'up'],
  ] as const)(
    'keeps a live remote pane and its draft across a re-read while its host is %s',
    async (_name, status) => {
      ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
      recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
      loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
      getOrCreateHostRuntime('host-1').getState().setStatus(status)
      fetchMock.mockImplementation(async (url: string) => {
        if (url === '/v0/projects') return listAnswer(projectOn('p-local', 'local'))
        if (url === 'crowbar://host-1/v0/projects') return unreachable()
        return new Response(JSON.stringify(PAGE), { status: 200 })
      })
      mount('w-remote')
      const composer = await screen.findByLabelText('composer')
      fireEvent.change(composer, { target: { value: 'half a thought' } })

      await act(() => useProjectDataStore.getState().fetch())

      expect(useProjectDataStore.getState().data.status).toBe('success')
      expect(screen.getByLabelText('composer')).toBe(composer)
      expect(composer).toHaveValue('half a thought')
    },
  )
})

describe('AgentChatPaneGate where no remote host was ever paired', () => {
  it('mounts at once and reads exactly as before, whatever the project list holds', async () => {
    mount()

    expect(await screen.findByTestId('chat-view')).toBeInTheDocument()
    await waitFor(() => expect(chatLedgerStore.getState().ledgers.c1?.status).toBe('loaded'))
    expect(chatReads()).toEqual(['/v0/projects/p-local/repos/r1/chats/c1/messages?limit=100'])
    expect(screen.queryByText(WAITING)).toBeNull()
  })
})
