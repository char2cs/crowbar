import { vi, beforeEach, describe, expect, it } from 'vitest'
import { holdShownChatFeeds } from '@/features/panes/lib/shown-chat-feeds'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { createLeaf } from '@/features/panes/utils/pane-layout'
import { IDBFactory } from 'fake-indexeddb'
import { getDB } from '@/lib/persistence/idb'
import { hydrateHostPlacements, startHostPlacementCache } from '@/lib/persistence/host-placements'
import { chatEntity, putEntities, resetEntities } from '@/__tests__/__fixtures__/sidebar-entities'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import { seedChats } from '@/__tests__/__fixtures__/agent-chat'
import { heldScopes, streamDaemon } from '@/__tests__/__fixtures__/stream-daemon'

const pane = (id: string, viewId: string, chatId: string) => ({
  ...Object.values(windowPaneStore.getState().panes)[0],
  id,
  viewId,
  chatId,
})

function showViews(active: 'view-a' | 'view-b') {
  const { panes } = windowPaneStore.getState()
  windowPaneStore.setState({
    panes: {
      ...panes,
      'pane-a': pane('pane-a', 'view-a', 'chat-a'),
      'pane-b': pane('pane-b', 'view-b', 'chat-b'),
    } as never,
    views: {
      'view-a': { id: 'view-a', projectId: 'p1', layout: createLeaf('pane-a') },
      'view-b': { id: 'view-b', projectId: 'p1', layout: createLeaf('pane-b') },
    } as never,
    viewOrder: ['view-a', 'view-b'],
    activeViewId: active,
    activePaneId: active === 'view-a' ? 'pane-a' : 'pane-b',
  })
}

beforeEach(() => {
  streamDaemon.answer = () => []
  streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
})

describe('holdShownChatFeeds', () => {
  it('subscribes the shown chat before any view mounts, and only while it is shown', async () => {
    seedChats([{ id: 'chat-a' }, { id: 'chat-b' }])
    showViews('view-a')
    await holdShownChatFeeds()
    await vi.waitFor(() => expect(heldScopes('chat:')).toEqual(['chat:chat-a']))
    showViews('view-b')
    await vi.waitFor(() => expect(heldScopes('chat:')).toEqual(['chat:chat-b']))
  })

  it('subscribes a chat the last run placed on a paired host before that host has listed', async () => {
    globalThis.indexedDB = new IDBFactory()
    resetEntities()
    await hydrateHostPlacements()
    const stop = startHostPlacementCache()
    pairRemoteHost('host-1')
    putEntities('chat', [chatEntity({ id: 'chat-a' })], 'host-1')
    await vi.waitFor(async () => expect(await (await getDB()).count('host-placements')).toBe(1))
    stop()
    resetEntities()
    await hydrateHostPlacements()
    showViews('view-a')

    await holdShownChatFeeds()

    await vi.waitFor(() => expect(heldScopes('chat:', 'host-1')).toEqual(['chat:chat-a']))
    resetHostWorld()
  })
})
