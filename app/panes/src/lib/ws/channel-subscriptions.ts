import { splitHostPath } from '@/lib/host-path'

import type { Channel } from './channel'
import type { ChannelContext } from './channel-context'
import { closeSocketQuietly } from './close-socket-quietly'
import { reportChannelGone } from './connection-store'
import { openChannel } from './open-channel'
import { refuseWaiters } from './refuse-waiters'
import type { StreamCallback } from './stream-callback'
import { isWebSocketCapable } from './url'

function cannotDial(ctx: ChannelContext, endpoint: string): boolean {
  if (!isWebSocketCapable()) return true
  const isLocal = ctx.ports.isLocalHost(splitHostPath(endpoint).hostId)
  return isLocal && !ctx.ports.hasLocalDaemon()
}

function retire(ctx: ChannelContext, current: Channel): void {
  if (current.socket) closeSocketQuietly(current.socket)
  ctx.channels.delete(current.endpoint)
  reportChannelGone(current.endpoint)
  refuseWaiters(current.waiters, current.endpoint, 'no subscriber left')
  ctx.parked.unwatchIfNoneParked(current.hostId)
}

function unsubscribe(ctx: ChannelContext, ch: Channel, cb: StreamCallback): void {
  ch.callbacks.delete(cb)
  if (ch.callbacks.size > 0) return
  const current = ctx.channels.get(ch.endpoint)
  const replaced = current?.callbacks !== ch.callbacks
  if (current && !replaced) retire(ctx, current)
  if (replaced && ch.socket) closeSocketQuietly(ch.socket)
}

export function subscribeTo(
  ctx: ChannelContext,
  endpoint: string,
  cb: StreamCallback,
): VoidFunction {
  if (cannotDial(ctx, endpoint)) {
    reportChannelGone(endpoint)
    return () => {}
  }
  const ch = ctx.channels.get(endpoint) ?? openChannel(ctx, endpoint)
  ch.callbacks.add(cb)
  return () => unsubscribe(ctx, ch, cb)
}
