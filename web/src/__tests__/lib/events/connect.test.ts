import '@/lib/transport/polyfill'
import { QueryClient } from '@tanstack/react-query'
import { connectDaemonEvents } from '@/lib/events/connect'
import { queryKeys } from '@/lib/queries/keys'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

describe('connectDaemonEvents', () => {
  let qc: QueryClient
  let disconnect: () => void

  beforeEach(() => {
    qc = new QueryClient()
    vi.spyOn(qc, 'invalidateQueries')
    disconnect = connectDaemonEvents(qc)
  })

  afterEach(() => {
    disconnect()
  })

  it('invalidates workspaces on workspace:updated', () => {
    window.__CROWBAR__.emit('workspace:updated')
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.workspaces.all,
    })
  })

  it('invalidates specific chat messages on chat:message', () => {
    window.__CROWBAR__.emit('chat:message', { chatId: 'chat-1' })
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.chats.messages('chat-1'),
    })
  })

  it('invalidates git status on git:changed', () => {
    window.__CROWBAR__.emit('git:changed', { workspaceId: 'ws-1' })
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.git.status('ws-1'),
    })
  })

  it('invalidates file tree on file:changed', () => {
    window.__CROWBAR__.emit('file:changed', { workspaceId: 'ws-1', path: '/src' })
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.files.tree('ws-1', '/src'),
    })
  })
})
