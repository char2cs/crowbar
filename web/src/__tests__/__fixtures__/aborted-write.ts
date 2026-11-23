import { vi } from 'vitest'

type StoreMethod = 'put' | 'delete' | 'count'

/** The next `method` call on any store finds its transaction aborted by the database, as a
 *  full quota or a closed connection does mid-write. Returns the spy. */
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

/** Collects every promise rejection nothing handled until `stop`; `flush` lets a late one land. */
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
