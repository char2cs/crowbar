interface MessageHandler {
  (event: { data: string }): void
}

interface ErrorHandler {
  (event: unknown): void
}

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
