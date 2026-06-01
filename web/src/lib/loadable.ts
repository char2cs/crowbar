export type Loadable<T> =
  | { status: 'idle' }
  | { status: 'loading'; staleData: T | null; staleAt: number | null }
  | { status: 'success'; data: T; fetchedAt: number }
  | { status: 'error'; error: Error; staleData: T | null; staleAt: number | null }

export const idle = (): Loadable<never> => ({ status: 'idle' })

export const loading = <T>(prev?: Loadable<T>): Loadable<T> => ({
  status: 'loading',
  staleData: dataOf(prev) ?? null,
  staleAt: fetchedAtOf(prev) ?? null,
})

export const success = <T>(data: T, at: number = Date.now()): Loadable<T> => ({
  status: 'success',
  data,
  fetchedAt: at,
})

export const failed = <T>(error: Error, prev?: Loadable<T>): Loadable<T> => ({
  status: 'error',
  error,
  staleData: dataOf(prev) ?? null,
  staleAt: fetchedAtOf(prev) ?? null,
})

export const dataOf = <T>(l?: Loadable<T>): T | undefined => {
  if (!l) return undefined
  if (l.status === 'success') return l.data
  if (l.status === 'loading' || l.status === 'error') return l.staleData ?? undefined
  return undefined
}

export const fetchedAtOf = <T>(l?: Loadable<T>): number | undefined => {
  if (!l) return undefined
  if (l.status === 'success') return l.fetchedAt
  if (l.status === 'loading' || l.status === 'error') return l.staleAt ?? undefined
  return undefined
}
