/** A caller of `opened()` waiting on its channel's socket. */
export interface Waiter {
  resolve(): void
  reject(reason: Error): void
}
