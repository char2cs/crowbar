import { shellConfig } from '@/sdk/shell-config'

const CONNECTING = 0
const OPEN = 1
const CLOSED = 3
const RELAY_SUBPROTOCOL = 'crowbar.relay.v1'
const TOKEN_SUBPROTOCOL_PREFIX = 'crowbar.token.'

type Relay = { url: string; token: string }

function isRelay(value: unknown): value is Relay {
  if (typeof value !== 'object' || value === null) return false
  const { url, token } = value as Record<string, unknown>
  return typeof url === 'string' && typeof token === 'string'
}

async function freshRelay(api: string, cached: Relay | null | undefined) {
  try {
    const response = await fetch(`${api}/__crowbar/relay`)
    if (!response.ok) return null
    const relay: unknown = await response.json()
    if (!isRelay(relay)) return null
    ;(window as unknown as { __CROWBAR__: { ws?: Relay } }).__CROWBAR__.ws = relay
    return relay
  } catch (error) {
    console.warn('crowbar: relay refresh failed, using the cached endpoint', error)
    return cached
  }
}

/** A WebSocket to `path` (`/v0/...`) on the pane's own host, through the shell's relay. The token
 *  travels in a subprotocol, never in the URL. The relay holds one token per pane and host, so
 *  other hosts are not reachable from here. Like `TauriWebSocket` it connects asynchronously: the
 *  shell is asked for a new token on every connect so a long-open pane can reconnect, then the
 *  native socket is dialled and its events are forwarded. A missing relay surfaces as `onerror`
 *  then `onclose`. */
export class ShellWebSocket {
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: ((event: unknown) => void) | null = null

  binaryType: BinaryType = 'blob'
  readyState: number = CONNECTING

  private socket: WebSocket | null = null

  constructor(path: string) {
    void this.connect(path)
  }

  private async connect(path: string): Promise<void> {
    const config = shellConfig()
    const relay = config && (await freshRelay(config.api, config.ws))
    if (this.readyState === CLOSED) return
    if (!relay) {
      this.readyState = CLOSED
      this.onerror?.(new Error('the shell issued this pane no relay endpoint'))
      this.onclose?.()
      return
    }
    this.dial(`${relay.url}${path}`, relay.token)
  }

  private dial(url: string, token: string): void {
    const socket = new WebSocket(url, [RELAY_SUBPROTOCOL, `${TOKEN_SUBPROTOCOL_PREFIX}${token}`])
    socket.binaryType = this.binaryType
    socket.onopen = () => {
      this.readyState = OPEN
      this.onopen?.()
    }
    socket.onmessage = (event: MessageEvent<string | ArrayBuffer>) => this.onmessage?.(event)
    socket.onerror = (event) => this.onerror?.(event)
    socket.onclose = () => {
      this.readyState = CLOSED
      this.onclose?.()
    }
    this.socket = socket
  }

  send(data: string): void {
    if (!this.socket) throw new Error('the shell socket is not connected yet')
    this.socket.send(data)
  }

  /** Before the relay answers this ends the attempt quietly, as a native close before open does. */
  close(): void {
    if (this.socket) {
      this.socket.close()
      return
    }
    this.readyState = CLOSED
  }
}
