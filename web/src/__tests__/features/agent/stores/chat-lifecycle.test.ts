import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { forgetEntities } from '@/core/entities/entity-store'
import { followChatLifecycle } from '@/features/agent/stores/chat-lifecycle'
import { chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { viewChatIds } from '@/features/panes/lib/view-state'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { getWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'
import {
  markProjectsSynced,
  putEntities,
  resetEntities,
  workspaceDTO,
} from '@/__tests__/__fixtures__/sidebar-entities'

const panes = () => windowPaneStore.getState().panes
const actions = () => windowPaneStore.getState().paneActions

let stop = () => {}

beforeEach(() => {
  resetEntities()
  resetChatStore()
  resetWindowPaneStoreForTests()
  __resetWorkspaceScopesForTest()
  stop = followChatLifecycle()
})

afterEach(() => stop())

describe('a chat the daemon deleted', () => {
  it('takes its feed and its pane with it', () => {
    writeChat({ id: 'c1' })
    openFeed('c1')
    actions().openChat('c1')

    forgetEntities('chat', ['c1'])

    expect(chatStore.getState().feeds.c1).toBeUndefined()
    expect(chatPaneIndex(panes()).has('c1')).toBe(false)
  })

  it('takes the prompts still waiting in its outbox with it', () => {
    writeChat({ id: 'c1' })
    const waiting = { requestId: 'r1', wsId: 'ws1', text: 'hi', state: 'failed' as const }
    chatStore.setState({ outbox: { c1: [waiting] } })

    forgetEntities('chat', ['c1'])

    expect(chatStore.getState().outbox.c1).toBeUndefined()
  })
})

describe('a runner leaving its chat', () => {
  it('frees the pane that showed that runner, and only that one', () => {
    writeChat({ id: 'c1', liveRunnerId: 'r1' })
    actions().openChat('c1', { runnerId: 'r1' })
    const paneId = chatPaneIndex(panes()).get('c1')!

    writeChat({ id: 'c1', title: 'renamed' })
    expect(panes()[paneId]?.runnerId).toBe('r1')

    writeChat({ id: 'c1', liveRunnerId: '' })
    expect(panes()[paneId]?.runnerId).toBeNull()
  })
})

describe('a chat that starts working with no pane on it', () => {
  it('is adopted in the background once the snapshot has landed', () => {
    markProjectsSynced()
    writeChat({ id: 'c1', projectId: 'p1', workspaceId: 'w1' })
    writeChat({ id: 'c1', working: true })

    const { viewOrder } = windowPaneStore.getState()
    expect(viewOrder.map((id) => viewChatIds(windowPaneStore.getState(), id))).toContainEqual([
      'c1',
    ])
  })

  it('is history, not a live change, while the first snapshot is still landing', () => {
    writeChat({ id: 'c1', projectId: 'p1', working: true })
    expect(chatPaneIndex(panes()).has('c1')).toBe(false)
  })
})

describe('a workspace entity', () => {
  it('records the scope its chat routes are built from, and forgets it when gone', () => {
    putEntities('workspace', [
      workspaceDTO({ id: 'ws-home', projectId: 'p2', repoId: '', owningChatId: 'c-home' }),
    ])
    expect(getWorkspaceScope('ws-home')).toMatchObject({
      projectId: 'p2',
      repoId: '',
      owningChatId: 'c-home',
    })

    forgetEntities('workspace', ['ws-home'])
    expect(getWorkspaceScope('ws-home')).toBeNull()
  })
})
