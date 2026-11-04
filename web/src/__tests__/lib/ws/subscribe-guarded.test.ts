import { beforeEach, describe, expect, it, vi } from 'vitest'

const { subscribe } = vi.hoisted(() => ({ subscribe: vi.fn() }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))

import { subscribeGuarded } from '@/lib/ws/subscribe-guarded'

beforeEach(() => {
  subscribe.mockReset()
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

describe('subscribeGuarded', () => {
  it('subscribes to the endpoint and hands back the manager’s unsubscribe', () => {
    const unsubscribe = vi.fn()
    subscribe.mockReturnValue(unsubscribe)
    const onFrame = vi.fn()

    const stop = subscribeGuarded(() => '/v0/chats/c1/ws', onFrame)

    expect(subscribe).toHaveBeenCalledWith('/v0/chats/c1/ws', onFrame)
    stop()
    expect(unsubscribe).toHaveBeenCalledTimes(1)
  })

  it('answers a no-op unsubscribe when the manager refuses the endpoint', () => {
    subscribe.mockImplementation(() => {
      throw new Error('remote hosts need the desktop app')
    })

    const stop = subscribeGuarded(() => 'crowbar://h1/v0/projects', vi.fn())

    expect(() => stop()).not.toThrow()
  })

  it('opens nothing when the endpoint cannot be built', () => {
    const stop = subscribeGuarded(() => {
      throw new Error('no host resolved')
    }, vi.fn())

    expect(subscribe).not.toHaveBeenCalled()
    expect(() => stop()).not.toThrow()
  })
})
