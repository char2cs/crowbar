import { vi } from 'vitest'

type StoreMethod = 'put' | 'delete' | 'count'

export function abortDuring(method: StoreMethod) {
  const original = IDBObjectStore.prototype[method] as (...args: unknown[]) => unknown
  return vi.spyOn(IDBObjectStore.prototype, method).mockImplementationOnce(function (
    this: IDBObjectStore,
    ...args: unknown[]
  ) {
    this.transaction.abort()
    return original.apply(this, args) as never
  })
}

export function watchUnhandled() {
  const seen: string[] = []
  const note = (reason: unknown) =>
    seen.push(`${(reason as Error).name}: ${(reason as Error).message}`)
  process.on('unhandledRejection', note)
  return {
    seen,
    async flush() {
      for (let i = 0; i < 3; i++) await new Promise((resolve) => setImmediate(resolve))
    },
    stop: () => process.off('unhandledRejection', note),
  }
}
