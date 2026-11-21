import { afterEach, describe, expect, it, vi } from 'vitest'
import type { PaneChannel } from '@/sdk/pane-channel'
import { getOwningChatId } from '@/lib/workspace-scope'
import { toastManager } from '@/lib/toast-manager'
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

  it.each([
    {
      name: 'shows a toast raised in the pane, even before boot, in the shell toast host',
      pane: 'core.editor',
      before: () =>
        toastManager.add({
          id: 'early',
          type: 'error',
          title: 'Open the pane',
          description: 'gone',
        }),
      after: (post: ReturnType<typeof vi.fn>) => {
        toastManager.add({ id: 'late', type: 'success', title: 'Saved' })
        const toasts = post.mock.calls
          .map(([text]) => JSON.parse(text as string) as { kind: string })
          .filter((frame) => frame.kind === 'shell.toast')
        expect(toasts).toEqual([
          expect.objectContaining({
            key: 'early',
            tone: 'error',
            title: 'Open the pane',
            description: 'gone',
          }),
          expect.objectContaining({ key: 'late', tone: 'success', title: 'Saved', pinned: false }),
        ])
      },
    },
    {
      name: 'records the owning chat of each workspace, which every chat-scoped route is addressed by',
      pane: 'core.review',
      before: () => undefined,
      after: () => {
        putEntities('workspace', [workspaceDTO({ id: 'ws1', projectId: 'p1', owningChatId: 'c1' })])
        expect(getOwningChatId('ws1')).toBe('c1')
      },
    },
  ])('$name', ({ pane, before, after }) => {
    const post = vi.fn()
    shell.channel = { post, onmessage: null } as unknown as PaneChannel
    document.body.innerHTML = '<div id="root"></div>'
    before()

    bootPane(pane)

    after(post)
  })
})
