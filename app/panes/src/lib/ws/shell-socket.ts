import { shellConfig } from '@/sdk/shell-config'

const RELAY_SUBPROTOCOL = 'crowbar.relay.v1'
const TOKEN_SUBPROTOCOL_PREFIX = 'crowbar.token.'

/** A native WebSocket to `path` (`/v0/...`) on the pane's own host, through the shell's relay. The token travels in a subprotocol, never in the URL. The relay holds one token per pane and host, so other hosts are not reachable from here. */
export function openShellSocket(path: string): WebSocket {
  const relay = shellConfig()?.ws
  if (!relay) throw new Error('the shell issued this pane no relay endpoint')
  return new WebSocket(`${relay.url}${path}`, [
    RELAY_SUBPROTOCOL,
    `${TOKEN_SUBPROTOCOL_PREFIX}${relay.token}`,
  ])
}
