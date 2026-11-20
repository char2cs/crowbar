import { afterEach, describe, expect, it, vi } from 'vitest'
import type { PaneChannel } from '@/sdk/pane-channel'
import { getOwningChatId } from '@/lib/workspace-scope'
import { putEntities, resetEntities, workspaceDTO } from '@/__tests__/__fixtures__/sidebar-entities'

const shell = vi.hoisted(() => ({ channel: null as PaneChannel | null }))
vi.mock('@/sdk/shell-channel', () => ({ shellChannel: () => shell.channel }))
vi.mock('@/core/stream/stream', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/core/stream/stream')>()),
  startStream: () => () => undefined,
}))
vi.mock('@/lib/events/connect', () => ({ connectDaemonEvents: () => () => undefined }))

const { bootPane } = await import('@/entries/pane-boot')

afterEach(() => {
  shell.channel = null
  document.body.innerHTML = ''
  resetEntities()
})

describe('bootPane', () => {
  it('refuses to run outside the Crowbar shell', () => {
    expect(() => bootPane('chat')).toThrow(/runs inside the Crowbar shell/)
  })

  it('records the owning chat of each workspace, which every chat-scoped route is addressed by', () => {
    shell.channel = { post: vi.fn(), onmessage: null } as unknown as PaneChannel
    document.body.innerHTML = '<div id="root"></div>'

    bootPane('core.review')
    putEntities('workspace', [workspaceDTO({ id: 'ws1', projectId: 'p1', owningChatId: 'c1' })])

    expect(getOwningChatId('ws1')).toBe('c1')
  })
})
