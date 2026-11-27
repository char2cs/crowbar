import { hostApi, shellConfig, type ShellConfig } from '@/sdk/shell-config'

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

async function freshRelay(config: ShellConfig, hostId: string) {
  const own = hostId === config.host
  const cached = own ? config.ws : null
  try {
    const response = await fetch(`${hostApi(config, hostId)}/__crowbar/relay`)
    if (!response.ok) return null
    const relay: unknown = await response.json()
    if (!isRelay(relay)) return null
    if (own) (window as unknown as { __CROWBAR__: { ws?: Relay } }).__CROWBAR__.ws = relay
    return relay
  } catch (error) {
    console.warn('crowbar: relay refresh failed, using the cached endpoint', error)
    return cached
  }
}

export class ShellWebSocket {
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: ((event: unknown) => void) | null = null

  binaryType: BinaryType = 'blob'
  readyState: number = CONNECTING

  private socket: WebSocket | null = null

  constructor(path: string, hostId: string) {
    void this.connect(path, hostId)
  }

  private async connect(path: string, hostId: string): Promise<void> {
    const config = shellConfig()
    const relay = config && (await freshRelay(config, hostId))
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

  close(): void {
    if (this.socket) {
      this.socket.close()
      return
    }
    this.readyState = CLOSED
  }
}
