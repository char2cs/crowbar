import { sendOn } from './channel-send'
import type { Channel } from './channel'
import type { ChannelContext } from './channel-context'
import { openedOn } from './channel-opened'
import { subscribeTo } from './channel-subscriptions'
import { onDrop } from './drop-listeners'
import { openChannel, retryOf } from './open-channel'
import { createParkedHosts } from './parked-hosts'
import type { WsHostPorts } from './ws-host-ports'
import type { WsManager } from './ws-manager'

export function createWSManager(ports: WsHostPorts): WsManager {
  const channels = new Map<string, Channel>()
  const ctx: ChannelContext = {
    channels,
    dropListeners: new Map(),
    ports,
    parked: createParkedHosts(channels, ports, (ch, delay) => ctx.reopen(ch, delay)),
    reopen: (ch, delay) => openChannel(ctx, ch.endpoint, retryOf(ch, delay)),
  }
  return {
    subscribe: (endpoint, cb) => subscribeTo(ctx, endpoint, cb),
    send: (endpoint, data) => sendOn(ctx, endpoint, data),
    opened: (endpoint) => openedOn(ctx, endpoint),
    onDrop: (endpoint, listener) => onDrop(ctx, endpoint, listener),
  }
}
