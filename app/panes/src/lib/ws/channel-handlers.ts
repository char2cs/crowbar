import { streamFailureOf } from '@/lib/api/errors'
import { noteDaemonChange } from '@/lib/transport/daemon-changes'

import type { Channel } from './channel'
import type { ChannelContext } from './channel-context'
import { reportChannelState, reportChannelFailure, reportChannelGone } from './connection-store'
import { noteSocketOpened, probeDaemon } from './daemon-probe'
import { notifyDrop } from './drop-listeners'
import { BASE_RECONNECT_DELAY_MS, nextReconnectDelay } from './reconnect-backoff'
import { refuseWaiters } from './refuse-waiters'
import type { Retry } from './retry'
import type { WsLike } from './ws-like'

function handleOpen(ch: Channel, socket: WsLike, ctx: ChannelContext, retry?: Retry) {
  return () => {
    ch.reconnectDelay = BASE_RECONNECT_DELAY_MS
    if (ctx.channels.get(ch.endpoint) !== ch) return
    ch.opened = true
    for (const data of ch.outbox.splice(0)) socket.send(data)
    if (ctx.ports.isLocalHost(ch.hostId)) noteSocketOpened()
    reportChannelState(ch.endpoint, true, ch.hostId)
    for (const waiter of ch.waiters.splice(0)) waiter.resolve()
    // SPEC: only a resumed open tells subscribers, not each attempt: a refetch while the daemon is down just fails.
    if (retry === undefined) return
    noteDaemonChange()
    ch.callbacks.forEach((cb) => cb({ reconnected: true }))
  }
}

function parseFrame(raw: string): unknown {
  try {
    return JSON.parse(raw)
  } catch {
    return raw
  }
}

function handleMessage(ch: Channel) {
  return (event: { data: string }) => {
    const parsed = parseFrame(event.data)
    noteDaemonChange()
    ch.callbacks.forEach((cb) => cb(parsed))
  }
}

function handleError(ch: Channel, ctx: ChannelContext) {
  return (failure: unknown) => {
    ch.failure = failure
    if (ctx.ports.isLocalHost(ch.hostId)) return
    const reason = streamFailureOf(failure, ch.hostId)
    const isLive = ctx.channels.get(ch.endpoint) === ch
    if (reason && isLive) reportChannelFailure(ch.endpoint, reason)
  }
}

function retryOrRetire(ch: Channel, ctx: ChannelContext): void {
  if (ch.callbacks.size > 0) {
    ctx.reopen(ch, nextReconnectDelay(ch.reconnectDelay))
    return
  }
  if (ctx.channels.get(ch.endpoint) === ch) {
    ctx.channels.delete(ch.endpoint)
    reportChannelGone(ch.endpoint)
  }
  refuseWaiters(ch.waiters, ch.endpoint, 'no subscriber left')
}

function handleClose(ch: Channel, ctx: ChannelContext) {
  return () => {
    const wasOpen = ch.opened
    ch.opened = false
    // SPEC: a stale socket closing after a reconnect must not flag the endpoint as down.
    const isLive = ctx.channels.get(ch.endpoint) === ch
    if (isLive) reportChannelState(ch.endpoint, false, ch.hostId)
    const refusedLocally = isLive && !wasOpen && ctx.ports.isLocalHost(ch.hostId)
    if (refusedLocally) void probeDaemon()
    refuseWaiters(ch.waiters, ch.endpoint, 'closed before it opened', ch.failure)
    if (ch.callbacks.size === 0) return
    if (isLive && wasOpen) notifyDrop(ctx, ch.endpoint)
    setTimeout(() => retryOrRetire(ch, ctx), ch.reconnectDelay)
  }
}

/** Wires `socket`'s events to `ch`; `retry` is set when the socket replaces a dropped one. */
export function bindSocket(ch: Channel, socket: WsLike, ctx: ChannelContext, retry?: Retry): void {
  socket.onopen = handleOpen(ch, socket, ctx, retry)
  socket.onmessage = handleMessage(ch)
  socket.onerror = handleError(ch, ctx)
  socket.onclose = handleClose(ch, ctx)
}
