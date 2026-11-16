/** What a subscriber hears: each parsed frame, and `{ reconnected: true }` when the stream is back. */
export interface StreamCallback {
  (data: unknown): void
}
