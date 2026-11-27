export interface Waiter {
  resolve(): void
  reject(reason: Error): void
}
