import type { Channel } from './channel'
import { BASE_RECONNECT_DELAY_MS } from './reconnect-backoff'
import type { WsHostPorts } from './ws-host-ports'

/** The remote hosts whose channels wait, parked, for the host to be dialable again. */
export interface ParkedHosts {
  /** Starts watching `hostId` for its return, once however many channels park on it. */
  watch(hostId: string): void
  unwatchIfNoneParked(hostId: string): void
}

function isParkedOn(ch: Channel, hostId: string): boolean {
  return ch.socket === null && ch.hostId === hostId
}

/** Reopens a host's parked channels, each at the base delay, when the host returns. */
export function createParkedHosts(
  channels: Map<string, Channel>,
  ports: WsHostPorts,
  reopen: (ch: Channel, reconnectDelay: number) => void,
): ParkedHosts {
  const watches = new Map<string, VoidFunction>()

  function unwatch(hostId: string): void {
    watches.get(hostId)?.()
    watches.delete(hostId)
  }

  function reopenParked(hostId: string): void {
    unwatch(hostId)
    const parked = [...channels.values()].filter((ch) => isParkedOn(ch, hostId))
    parked.forEach((ch) => reopen(ch, BASE_RECONNECT_DELAY_MS))
  }

  function watch(hostId: string): void {
    if (watches.has(hostId)) return
    watches.set(
      hostId,
      ports.onHostReady(hostId, () => reopenParked(hostId)),
    )
  }

  function unwatchIfNoneParked(hostId: string): void {
    const stillParked = [...channels.values()].some((ch) => isParkedOn(ch, hostId))
    if (!stillParked) unwatch(hostId)
  }

  return { watch, unwatchIfNoneParked }
}
