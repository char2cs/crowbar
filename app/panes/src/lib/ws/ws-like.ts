interface MessageHandler {
  (event: { data: string }): void
}

interface ErrorHandler {
  (event: unknown): void
}

/** The subset of the WebSocket surface the manager drives: the native `WebSocket`, the desktop
 *  `TauriWebSocket` shim and the shell relay all satisfy it, so the manager is transport-agnostic. */
export interface WsLike {
  onopen: VoidFunction | null
  onmessage: MessageHandler | null
  onclose: VoidFunction | null
  onerror: ErrorHandler | null
  readyState: number
  send(data: string): void
  close(): void
  addEventListener?(type: 'open', listener: VoidFunction, options?: { once: boolean }): void
}
