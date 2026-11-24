import { IDBFactory } from 'fake-indexeddb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { attachFeed } from '@/features/agent/stores/chat-feed'
import { chatStore } from '@/features/agent/stores/chat-store'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { streamDaemon, streamDaemonFor } from '@/__tests__/__fixtures__/stream-daemon'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import { wsManager } from '@/core/stream/ws-manager'

const SCOPE = 'chat:chat-1'
const feed = () => chatStore.getState().feeds['chat-1']
const subs = () => streamDaemon.sent.filter((r) => r.op === 'sub' && r.scope === SCOPE)
const held: (() => void)[] = []

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
  chatStore.setState({ feeds: {} })
  streamDaemon.answer = () => []
  streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
  streamDaemon.sent.length = 0
  vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
})

describe("a paired host's chat", () => {
  const host1 = streamDaemonFor('host-1')
  const subsOn = () => host1.sent.filter((r) => r.op === 'sub' && r.scope === SCOPE)
  afterEach(() => {
    held.splice(0).forEach((detach) => detach())
    vi.useRealTimers()
    vi.restoreAllMocks()
    resetHostWorld()
  })

  it('removing the host drops the feeds it kept', async () => {
    pairRemoteHost('host-1')
    await Promise.resolve()
    host1.answer = () => []
    const detach = attachFeed('host-1', 'chat-1')
    await vi.waitFor(() => expect(subsOn()).toHaveLength(1))
    host1.deliver({ t: 'synced', scope: SCOPE })
    detach()
    expect(feed()).toBeDefined()

    useHostConfigStore.getState().removeHost('host-1')
    expect(feed()).toBeUndefined()
  })

  it('subs on its host and resumes from its seq when that host reconnects, not when local does', async () => {
    pairRemoteHost('host-1')
    await Promise.resolve()
    host1.answer = () => []
    held.push(attachFeed('host-1', 'chat-1'))
    await vi.waitFor(() => expect(subsOn()).toHaveLength(1))
    expect(subsOn()).toEqual([{ op: 'sub', scope: SCOPE, since: -1500 }])
    expect(subs()).toEqual([])
    const l = ledger()
    host1.deliver({ t: 'row', scope: SCOPE, row: l.prompt('r1', 'hi') })
    host1.deliver({ t: 'synced', scope: SCOPE, seq: 1 })

    streamDaemon.drop()
    streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
    expect(subs()).toEqual([])
    host1.drop()
    host1.deliver({ t: 'ready', epoch: 'h1' })
    expect(subsOn().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: 1 })
  })

  // The stream's open to host-1 fails the way the desktop bridge rejects it, before any sync.
  const openFails = (cause: string) => {
    host1.answer = () => []
    vi.spyOn(wsManager, 'opened').mockRejectedValue(
      new Error('host-1~/v0/stream: closed before it opened', { cause }),
    )
  }

  it('says the host presents another identity when the open names it', async () => {
    pairRemoteHost('host-1', 'Studio')
    openFails('host-identity-changed: the pinned certificate does not match')
    held.push(attachFeed('host-1', 'chat-1'))
    await vi.waitFor(() => expect(feed()?.status).toBe('error'))
    expect(feed()?.error).toBe(
      'Studio is presenting a different identity — confirm it before pairing again',
    )
  })

  it('says the host must be paired again when its status records the refusal', async () => {
    pairRemoteHost('host-1', 'Studio')
    getOrCreateHostRuntime('host-1').getState().setStatus('needs-repair')
    openFails('ws upgrade: HTTP error: 401 Unauthorized')
    held.push(attachFeed('host-1', 'chat-1'))
    await vi.waitFor(() => expect(feed()?.status).toBe('error'))
    expect(feed()?.error).toBe('Studio needs to be paired again')
  })
})
