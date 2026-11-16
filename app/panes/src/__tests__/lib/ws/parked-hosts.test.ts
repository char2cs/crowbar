import { describe, it, expect, vi } from 'vitest'
import { createParkedHosts } from '@/lib/ws/parked-hosts'
import { fakeChannel, fakePorts } from '@/__tests__/__fixtures__/ws-context'

function setup() {
  const parkedCh = fakeChannel({ socket: null, endpoint: '/p', hostId: 'h1' })
  const liveCh = fakeChannel({ endpoint: '/l', hostId: 'h1' })
  const channels = new Map([
    [parkedCh.endpoint, parkedCh],
    [liveCh.endpoint, liveCh],
  ])
  const unwatch = vi.fn()
  let ready: () => void = () => {}
  const onHostReady = vi.fn((_host: string, listener: () => void) => {
    ready = listener
    return unwatch
  })
  const reopen = vi.fn()
  const parked = createParkedHosts(channels, fakePorts({ onHostReady }), reopen)
  return { parked, parkedCh, onHostReady, reopen, unwatch, channels, fire: () => ready() }
}

describe('createParkedHosts', () => {
  it('watches a host once however many channels park on it', () => {
    const { parked, onHostReady } = setup()
    parked.watch('h1')
    parked.watch('h1')
    expect(onHostReady).toHaveBeenCalledOnce()
  })

  it('reopens only the parked channels of the host that returned, at the base delay', () => {
    const { parked, parkedCh, reopen, fire, unwatch } = setup()
    parked.watch('h1')
    fire()
    expect(reopen).toHaveBeenCalledExactlyOnceWith(parkedCh, 1000)
    expect(unwatch).toHaveBeenCalledOnce()
  })

  it.each([
    [false, 0],
    [true, 1],
  ])('with its parked channel gone=%s the watch is dropped %i times', (gone, dropped) => {
    const { parked, parkedCh, channels, unwatch } = setup()
    parked.watch('h1')
    if (gone) channels.delete(parkedCh.endpoint)
    parked.unwatchIfNoneParked('h1')
    expect(unwatch).toHaveBeenCalledTimes(dropped)
  })
})
