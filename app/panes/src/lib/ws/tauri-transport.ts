import { Channel, invoke } from '@tauri-apps/api/core'

const WS_CLOSE_SENTINEL = '\u0000crowbar-ws-close'

export class TauriWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 3

  onopen: (() => void) | null = null
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: ((event: unknown) => void) | null = null

  readyState: number = TauriWebSocket.CONNECTING

  private readonly connId: string
  private closed = false
  private early: (string | ArrayBuffer)[] | null = []

  constructor(path: string, options?: { idleTimeoutMs?: number; hostId?: string }) {
    this.connId = crypto.randomUUID()
    const hostId = options?.hostId ?? 'local'

    const channel = new Channel<string | ArrayBuffer>()
    channel.onmessage = (frame) => {
      if (this.early) this.early.push(frame)
      else this.deliver(frame)
    }

    void invoke('ws_open', {
      connId: this.connId,
      path,
      onMessage: channel,
      idleTimeoutMs: options?.idleTimeoutMs ?? null,
      hostId: hostId === 'local' ? undefined : hostId,
    }).then(
      () => {
        const early = this.early ?? []
        this.early = null
        if (this.closed) {
          void invoke('ws_close', { connId: this.connId })
          return
        }
        this.readyState = TauriWebSocket.OPEN
        this.onopen?.()
        for (const frame of early) this.deliver(frame)
      },
      (err: unknown) => {
        this.early = null
        this.readyState = TauriWebSocket.CLOSED
        if (this.closed) return
        this.onerror?.(err)
        this.onclose?.()
      },
    )
  }

  private deliver(frame: string | ArrayBuffer): void {
    if (this.readyState !== TauriWebSocket.OPEN) return
    if (frame === WS_CLOSE_SENTINEL) {
      this.readyState = TauriWebSocket.CLOSED
      this.onclose?.()
      return
    }
    this.onmessage?.({ data: frame })
  }

  send(data: string): void {
    void invoke('ws_send', { connId: this.connId, data })
  }

  close(): void {
    const wasConnecting = this.readyState === TauriWebSocket.CONNECTING
    this.readyState = TauriWebSocket.CLOSED
    this.closed = true
    if (!wasConnecting) {
      void invoke('ws_close', { connId: this.connId })
    }
    this.onclose?.()
  }
}
