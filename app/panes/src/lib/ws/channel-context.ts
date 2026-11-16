import type { Channel } from './channel'
import type { ParkedHosts } from './parked-hosts'
import type { WsHostPorts } from './ws-host-ports'

/** The state one manager's channel modules share. */
export interface ChannelContext {
  channels: Map<string, Channel>
  dropListeners: Map<string, Set<VoidFunction>>
  ports: WsHostPorts
  parked: ParkedHosts
  /** Opens `ch`'s endpoint again, taking over its subscribers and waiters. */
  reopen(ch: Channel, reconnectDelay: number): void
}
