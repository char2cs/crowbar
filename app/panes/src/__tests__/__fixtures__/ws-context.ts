import { vi } from 'vitest'
import type { Channel } from '@/lib/ws/channel'
import type { ChannelContext } from '@/lib/ws/channel-context'
import type { WsHostPorts } from '@/lib/ws/ws-host-ports'
import type { WsLike } from '@/lib/ws/ws-like'

export function fakeSocket(readyState = 1): WsLike {
  return {
    onopen: null,
    onmessage: null,
    onclose: null,
    onerror: null,
    readyState,
    send: vi.fn(),
    close: vi.fn(),
  }
}

export function fakeChannel(over: Partial<Channel> = {}): Channel {
  return {
    socket: fakeSocket(),
    callbacks: new Set(),
    opened: false,
    reconnectDelay: 1000,
    endpoint: '/v0/ws/x',
    hostId: 'local',
    waiters: [],
    outbox: [],
    ...over,
  }
}

export function fakePorts(over: Partial<WsHostPorts> = {}): WsHostPorts {
  return {
    isLocalHost: (hostId) => hostId === 'local',
    mayDial: () => true,
    hasLocalDaemon: () => true,
    onHostReady: () => () => {},
    ...over,
  }
}

export function fakeContext(over: Partial<ChannelContext> = {}): ChannelContext {
  return {
    channels: new Map(),
    dropListeners: new Map(),
    ports: fakePorts(),
    parked: { watch: vi.fn(), unwatchIfNoneParked: vi.fn() },
    reopen: vi.fn(),
    ...over,
  }
}
