// WebSocket shim over the Tauri unix-socket Channel bridge: on desktop the browser WebSocket can't
// reach the daemon (`crowbar://`), so Rust dials, upgrades and bridges frames (ws_bridge.rs).
// Frames surface as `{ data }` like a native MessageEvent: text as string, binary as ArrayBuffer.

import { Channel, invoke } from '@tauri-apps/api/core'

// Mirrors ws_bridge::WS_CLOSE_SENTINEL — pushed down the Channel when the daemon
// closes the stream (restart/timeout/error). The NUL prefix cannot collide with
// a real JSON DTO frame, so the shim treats it as a close, not a message.
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
  // Set by close() so a teardown while CONNECTING is honoured once ws_open resolves: Rust registers
  // the connection only after the dial+upgrade, so an earlier ws_close is a no-op there and would
  // leak the daemon FD and two tokio tasks per StrictMode double-mount.
  private closed = false
  /** Frames forwarded before ws_open resolved; null once it has. */
  private early: (string | ArrayBuffer)[] | null = []

  /** `idleTimeoutMs`: close as half-open when the daemon sends nothing, not even a ping, for this
   *  long (only for streams it pings; unset = never). `hostId`: the remote host to dial; omitted
   *  or 'local' dials the local daemon. */
  constructor(path: string, options?: { idleTimeoutMs?: number; hostId?: string }) {
    this.connId = crypto.randomUUID()
    const hostId = options?.hostId ?? 'local'

    // The Channel carries each raw frame the Rust reader forwards, which starts
    // before ws_open's reply: frames that beat it wait in `early`, so onopen
    // always precedes onmessage as the WebSocket contract promises.
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
        // Closed while CONNECTING: the Rust connection is only now registered,
        // so issue the deferred ws_close to tear it down. Per the WebSocket
        // contract a close before open yields no onopen — just a clean teardown.
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

  /** One frame of an open socket; once closed, frames still in flight are dropped. */
  private deliver(frame: string | ArrayBuffer): void {
    if (this.readyState !== TauriWebSocket.OPEN) return
    // A daemon-side close (restart/timeout/error) arrives as the sentinel:
    // surface it as a close so wsManager reconnects and the §6 cache re-seeds.
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
    // While CONNECTING the Rust side isn't registered, so ws_close would be a no-op; the ws_open
    // handler issues it. Close eagerly only when there is something to close.
    if (!wasConnecting) {
      void invoke('ws_close', { connId: this.connId })
    }
    this.onclose?.()
  }
}
