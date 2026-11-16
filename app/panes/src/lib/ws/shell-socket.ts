import { shellConfig } from '@/sdk/shell-config'

const HTTP_OK = 200
const RELAY_SUBPROTOCOL = 'crowbar.relay.v1'
const TOKEN_SUBPROTOCOL_PREFIX = 'crowbar.token.'

type Relay = { url: string; token: string }

function freshRelay(api: string, cached: Relay | null | undefined): Relay | null | undefined {
  try {
    const lookup = new XMLHttpRequest()
    lookup.open('GET', `${api}/__crowbar/relay`, false)
    lookup.send()
    if (lookup.status !== HTTP_OK) return cached
    const relay = JSON.parse(lookup.responseText) as Relay
    ;(window as unknown as { __CROWBAR__: { ws?: Relay } }).__CROWBAR__.ws = relay
    return relay
  } catch (error) {
    console.warn('crowbar: relay refresh failed, using the cached endpoint', error)
    return cached
  }
}

/** A native WebSocket to `path` (`/v0/...`) on the pane's own host, through the shell's relay. The token travels in a subprotocol, never in the URL. The relay holds one token per pane and host, so other hosts are not reachable from here. */
export function openShellSocket(path: string): WebSocket {
  const config = shellConfig()
  const relay = config && freshRelay(config.api, config.ws)
  if (!relay) throw new Error('the shell issued this pane no relay endpoint')
  return new WebSocket(`${relay.url}${path}`, [
    RELAY_SUBPROTOCOL,
    `${TOKEN_SUBPROTOCOL_PREFIX}${relay.token}`,
  ])
}
