import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PaneChannel } from '@/sdk/pane-channel'
import { PaneClient } from '@/sdk/pane-client'
import { PaneRequestError } from '@/sdk/pane-request-error'

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
      'opens a pane',
      (client: PaneClient) => client.open('core.files', { spaceId: 's1' }),
      { kind: 'pane.open', typeId: 'core.files', params: { spaceId: 's1' } },
    ],
    [
      'asks the shell to store a setting',
      (client: PaneClient) => client.setSetting('showMinimap', true),
      { kind: 'settings.set', version: 1, key: 'showMinimap', value: true },
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

  it('reads a setting and follows its changes', async () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const read = client.getSetting('editor.fontSize')
    const { id } = pane.posted[0] as { id: number }
    pane.deliver({ sdk: 1, kind: 'response', id, result: 14 })
    await expect(read).resolves.toBe(14)

    const changed = vi.fn()
    const stop = client.subscribeSetting('editor.fontSize', changed)
    const subscription = pane.posted[1] as { id: number; kind: string }
    expect(subscription.kind).toBe('settings.subscribe')
    pane.deliver({
      sdk: 1,
      kind: 'settings.changed',
      id: subscription.id,
      key: 'editor.fontSize',
      value: 16,
    })
    expect(changed).toHaveBeenCalledWith(16)
    stop()
    expect(pane.posted[2]).toEqual({ sdk: 1, kind: 'unsubscribe', id: subscription.id })
  })

  it('hears the shell settings of the version it speaks and no other', () => {
    const client = new PaneClient(pane.channel, 'core.chat')
    const heard = vi.fn()
    client.onSettings(heard)
    pane.deliver({ sdk: 1, kind: 'settings', version: 1, values: { wordWrap: true } })
    pane.deliver({ sdk: 1, kind: 'settings', version: 2, values: { wordWrap: false } })
    expect(heard.mock.calls).toEqual([[{ wordWrap: true }]])
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
