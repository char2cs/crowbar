import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PaneChannel } from '@/sdk/pane-channel'
import { PaneClient } from '@/sdk/pane-client'
import { PaneRequestError } from '@/sdk/pane-request-error'
import { REQUEST_TIMEOUT_MS } from '@/sdk/sdk-limits'

function fakeChannel() {
  const posted: Record<string, unknown>[] = []
  const channel: PaneChannel = {
    post: (text) => posted.push(JSON.parse(text) as Record<string, unknown>),
    onmessage: null,
  }
  const deliver = (frame: object) => channel.onmessage?.(JSON.stringify(frame))
  return { channel, posted, deliver }
}

let pane: ReturnType<typeof fakeChannel>
beforeEach(() => {
  pane = fakeChannel()
})

describe('PaneClient', () => {
  it.each([
    [
      'registers its pane type with the SDK version',
      (client: PaneClient) => client.register(),
      { kind: 'pane.register', typeId: 'core.chat', sdkVersion: '1.0' },
    ],
    [
      'sends the title cleaned for the shell tab',
      (client: PaneClient) => client.setTitle('  Fix the build\n'),
      { kind: 'pane.setTitle', title: 'Fix the build' },
    ],
    [
      'saves state',
      (client: PaneClient) => client.setState({ scroll: 4 }),
      { kind: 'pane.setState', state: { scroll: 4 } },
    ],
    [
      'reports unsaved work',
      (client: PaneClient) => client.markDirty(),
      { kind: 'pane.setDirty', dirty: true },
    ],
    [
      'reports no unsaved work',
      (client: PaneClient) => client.markClean(),
      { kind: 'pane.setDirty', dirty: false },
    ],
    [
      'opens a pane',
      (client: PaneClient) => client.open('core.files', { spaceId: 's1' }),
      { kind: 'pane.open', typeId: 'core.files', params: { spaceId: 's1' } },
    ],
    [
      'opens a file as the preview tab',
      (client: PaneClient) =>
        client.open('core.editor', {}, { file: 'src/pricing.ts', mode: 'preview' }),
      {
        kind: 'pane.open',
        typeId: 'core.editor',
        params: {},
        file: 'src/pricing.ts',
        mode: 'preview',
      },
    ],
    [
      'asks the shell to store a setting',
      (client: PaneClient) => client.setSetting('showMinimap', true),
      { kind: 'settings.set', version: 1, key: 'showMinimap', value: true },
    ],
    [
      'marks review files viewed',
      (client: PaneClient) =>
        client.setReviewViewed({
          files: [{ path: 'src/a.ts', signature: 'M||1|0' }],
          viewed: true,
        }),
      {
        kind: 'review.setViewed',
        files: [{ path: 'src/a.ts', signature: 'M||1|0' }],
        viewed: true,
      },
    ],
    [
      'runs a command',
      (client: PaneClient) => client.runCommand('workbench.settings'),
      { kind: 'command.run', command: 'workbench.settings' },
    ],
  ])('%s', (_name, act, frame) => {
    act(new PaneClient(pane.channel, 'core.chat'))
    expect(pane.posted).toEqual([{ sdk: 1, ...frame }])
  })

  it('hands lifecycle messages to its listeners until they stop', () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const heard = vi.fn()
    const stop = client.onLifecycle(heard)
    const mount = {
      sdk: 1,
      kind: 'lifecycle',
      paneId: 'p1',
      event: 'mount',
      params: { threadId: 't1' },
      state: null,
    }
    pane.deliver(mount)
    stop()
    pane.deliver({ ...mount, event: 'activate' })
    expect(heard).toHaveBeenCalledTimes(1)
    expect(heard.mock.calls[0]![0]).toMatchObject({ paneId: 'p1', event: 'mount' })
  })

  it('hands the shell save request to its listeners until they stop', () => {
    const client = new PaneClient(pane.channel, 'core.editor')
    const heard = vi.fn()
    const stop = client.onSave(heard)
    pane.deliver({ sdk: 1, kind: 'pane.save' })
    stop()
    pane.deliver({ sdk: 1, kind: 'pane.save' })
    expect(heard).toHaveBeenCalledTimes(1)
  })

  it('refuses state over the shell limit without sending it', () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    expect(() => client.setState('x'.repeat(64 * 1024))).toThrow(RangeError)
    expect(pane.posted).toHaveLength(0)
  })

  it('resolves a request with the result of its response and rejects with the error', async () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const ok = client.request('ping', { n: 1 })
    const bad = client.request('boom')
    const [first, second] = pane.posted as { id: number; method: string }[]
    expect(first).toMatchObject({ kind: 'request', method: 'ping', params: { n: 1 } })
    pane.deliver({
      sdk: 1,
      kind: 'response',
      id: second!.id,
      error: { code: 'denied', message: 'no' },
    })
    pane.deliver({ sdk: 1, kind: 'response', id: first!.id, result: null })
    await expect(ok).resolves.toBeNull()
    await expect(bad).rejects.toMatchObject({ code: 'denied', message: 'no' })
    await expect(bad).rejects.toBeInstanceOf(PaneRequestError)
  })

  it('subscribes once per event and unsubscribes with the id it subscribed under', () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const a = vi.fn()
    const b = vi.fn()
    const stopA = client.subscribe('build.done', a)
    const stopB = client.subscribe('build.done', b)
    const id = (pane.posted[0] as { id: number }).id
    expect(pane.posted).toHaveLength(1)
    pane.deliver({
      sdk: 1,
      kind: 'extension.event',
      extensionId: 'x',
      name: 'build.done',
      payload: { ok: true },
    })
    expect(a).toHaveBeenCalledWith({ ok: true })
    expect(b).toHaveBeenCalledWith({ ok: true })
    stopA()
    expect(pane.posted).toHaveLength(1)
    stopB()
    expect(pane.posted[1]).toEqual({ sdk: 1, kind: 'unsubscribe', id })
  })

  it('rejects a request the shell never answers instead of hanging', async () => {
    vi.useFakeTimers()
    try {
      const client = new PaneClient(pane.channel, 'core.chat')
      const silent = client.request('ping')
      const settled = expect(silent).rejects.toMatchObject({ code: 'Timeout' })
      await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS)
      await settled
      const { id } = pane.posted[0] as { id: number }
      pane.deliver({ sdk: 1, kind: 'response', id, result: 1 })
    } finally {
      vi.useRealTimers()
    }
  })

  it('reports a frame the shell refused without dropping the pane', () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const heard = vi.fn()
    client.onLifecycle(heard)
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    pane.deliver({ sdk: 1, kind: 'protocol.error', message: 'frame too large' })
    expect(error).toHaveBeenCalledWith('crowbar: the shell refused a frame', 'frame too large')
    error.mockRestore()
  })

  it('hears the shell settings of the version it speaks and no other', () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const heard = vi.fn()
    client.onSettings(heard)
    pane.deliver({ sdk: 1, kind: 'settings', version: 1, values: { wordWrap: true } })
    pane.deliver({ sdk: 1, kind: 'settings', version: 2, values: { wordWrap: false } })
    expect(heard.mock.calls).toEqual([[{ wordWrap: true }]])
  })

  it('hears whether its chat fills its group', () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const heard = vi.fn()
    client.onGroup(heard)
    pane.deliver({ sdk: 1, kind: 'group', chatFills: true })
    pane.deliver({ sdk: 1, kind: 'group', chatFills: false })
    expect(heard.mock.calls).toEqual([[true], [false]])
  })

  it('hears the window’s chat zoom', () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const heard = vi.fn()
    client.onChatZoom(heard)
    pane.deliver({ sdk: 1, kind: 'chat.zoom', percent: 120 })
    expect(heard.mock.calls).toEqual([[120]])
  })

  it('hears the viewed map and the files to reveal the shell sends', () => {
    const client = new PaneClient(pane.channel, 'core.review')
    const viewed = vi.fn()
    const reveal = vi.fn()
    client.onReviewViewed(viewed)
    client.onReviewReveal(reveal)
    pane.deliver({ sdk: 1, kind: 'review.viewed', workspace: 'w1', viewed: { 'a.ts': 's' } })
    pane.deliver({ sdk: 1, kind: 'review.reveal', path: 'src/a.ts' })
    expect(viewed.mock.calls).toEqual([[{ workspace: 'w1', viewed: { 'a.ts': 's' } }]])
    expect(reveal.mock.calls).toEqual([['src/a.ts']])
  })

  it('ignores text that is not a frame', () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const heard = vi.fn()
    client.onLifecycle(heard)
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    pane.channel.onmessage?.('not json')
    expect(heard).not.toHaveBeenCalled()
    expect(error).toHaveBeenCalled()
    error.mockRestore()
  })
})
