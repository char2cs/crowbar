import { afterEach, describe, expect, it, vi } from 'vitest'
import type { LifecycleMessage } from '@/sdk/lifecycle-message'
import type { PaneClient } from '@/sdk/pane-client'
import { chatSession } from '@/entries/chat-session'
import { draftOfPane } from '@/components/layout/draft-thread'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getWorkspaceScope } from '@/lib/workspace-scope'

vi.mock('@/entries/chat-pane-host', () => ({ ChatPaneHost: () => null }))

const { ChatPaneMount } = await import('@/entries/chat-pane-mount')

type GroupListener = Parameters<PaneClient['onGroup']>[0]

let tellGroup: GroupListener = () => {}

const setParams = vi.fn()

function startedMount() {
  let deliver: (message: LifecycleMessage) => void = () => {}
  const onLifecycle = (listener: (message: LifecycleMessage) => void) => {
    deliver = listener
    return () => {}
  }
  const onGroup = (listener: GroupListener) => {
    tellGroup = listener
    return () => {}
  }
  const client = { onLifecycle, onGroup, setParams } as unknown as PaneClient
  new ChatPaneMount(document.createElement('div'), client).start()
  return (event: LifecycleMessage['event'], params: Record<string, string> = {}) =>
    deliver({ paneId: 'p1', event, params, state: null })
}

afterEach(() => {
  setParams.mockReset()
  chatSession.setState({ chatId: null, active: false, fillsGroup: false })
})

describe('ChatPaneMount', () => {
  it('shows the chat named by threadId on mount and follows activation', () => {
    const send = startedMount()
    send('mount', { threadId: 't1' })
    expect(chatSession.getState()).toEqual({ chatId: 't1', active: false, fillsGroup: false })
    send('activate')
    expect(chatSession.getState().active).toBe(true)
    send('deactivate')
    expect(chatSession.getState().active).toBe(false)
  })

  it.each(['park', 'destroy'] as const)('forgets the chat on %s', (event) => {
    const send = startedMount()
    send('mount', { threadId: 't1' })
    send(event)
    expect(chatSession.getState().chatId).toBeNull()
  })

  it('follows whether the chat fills its group and forgets it with the chat', () => {
    const send = startedMount()
    send('mount', { threadId: 't1' })
    tellGroup(true)
    expect(chatSession.getState().fillsGroup).toBe(true)
    send('park')
    expect(chatSession.getState().fillsGroup).toBe(false)
  })

  it('refuses a mount without a threadId', () => {
    const send = startedMount()
    expect(() => send('mount')).toThrow(/threadId/)
  })

  it('records the scope of the workspace a draft will run in, which no chat of the page has streamed', () => {
    const send = startedMount()
    send('mount', { threadId: 'draft-2', workspaceId: 'ws-2', projectId: 'p1', repoId: 'r1' })
    expect(getWorkspaceScope('ws-2')).toMatchObject({ projectId: 'p1', repoId: 'r1', wsId: 'ws-2' })
    send('mount', { threadId: 'draft-3', workspaceId: 'ws-3', projectId: 'p1' })
    expect(getWorkspaceScope('ws-3')).toMatchObject({ projectId: 'p1', repoId: '', wsId: 'ws-3' })
  })

  it('mounts a draft where its chat will be made and re-points the shell at the chat it becomes', () => {
    const send = startedMount()
    send('mount', { threadId: 'draft-1', workspaceId: 'ws-1', projectId: 'p1', parentId: 'bo' })
    expect(draftOfPane(ROOT_PANE_ID)).toEqual({
      paneId: ROOT_PANE_ID,
      draftId: 'draft-1',
      projectId: 'p1',
      workspaceId: 'ws-1',
      parentId: 'bo',
    })
    expect(setParams).not.toHaveBeenCalled()
    windowPaneStore.getState().paneActions.retargetPane(ROOT_PANE_ID, 'c9', null)
    expect(chatSession.getState().chatId).toBe('c9')
    expect(setParams.mock.calls).toEqual([[{ threadId: 'c9' }]])
  })
})
