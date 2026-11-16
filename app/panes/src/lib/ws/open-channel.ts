import { splitHostPath } from '@/lib/host-path'

import { bindSocket } from './channel-handlers'
import type { Channel } from './channel'
import type { ChannelContext } from './channel-context'
import { reportChannelState } from './connection-store'
import { createTransport } from './create-transport'
import { BASE_RECONNECT_DELAY_MS } from './reconnect-backoff'
import type { Retry } from './retry'
import type { WsLike } from './ws-like'

function newChannel(
  endpoint: string,
  hostId: string,
  socket: WsLike | null,
  retry?: Retry,
): Channel {
  return {
    socket,
    callbacks: retry?.callbacks ?? new Set(),
    waiters: retry?.waiters ?? [],
    opened: false,
    reconnectDelay: retry?.reconnectDelay ?? BASE_RECONNECT_DELAY_MS,
    endpoint,
    hostId,
    outbox: [],
  }
}

function park(ctx: ChannelContext, endpoint: string, hostId: string, retry?: Retry): Channel {
  const ch = newChannel(endpoint, hostId, null, retry)
  ctx.channels.set(endpoint, ch)
  reportChannelState(endpoint, false, hostId)
  ctx.parked.watch(hostId)
  return ch
}

/** The retry that reopens `ch`'s endpoint at `reconnectDelay`, taking over its subscribers and waiters. */
export function retryOf(ch: Channel, reconnectDelay: number): Retry {
  return { callbacks: ch.callbacks, waiters: ch.waiters.splice(0), reconnectDelay }
}

/** Dials `endpoint`, or parks it while its host may not be dialled. */
export function openChannel(ctx: ChannelContext, endpoint: string, retry?: Retry): Channel {
  const { hostId } = splitHostPath(endpoint)
  if (!ctx.ports.mayDial(hostId)) return park(ctx, endpoint, hostId, retry)
  const socket = createTransport(endpoint, ctx.ports.isLocalHost)
  const ch = newChannel(endpoint, hostId, socket, retry)
  bindSocket(ch, socket, ctx, retry)
  ctx.channels.set(endpoint, ch)
  return ch
}
