import type { Channel } from './channel'
import type { ParkedHosts } from './parked-hosts'
import type { WsHostPorts } from './ws-host-ports'

export interface ChannelContext {
  channels: Map<string, Channel>
  dropListeners: Map<string, Set<VoidFunction>>
  ports: WsHostPorts
  parked: ParkedHosts
  reopen(ch: Channel, reconnectDelay: number): void
}
