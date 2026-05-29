import './types'

if (typeof window !== 'undefined' && !window.__CROWBAR__) {
  const listeners = new Map<string, Set<(payload: unknown) => void>>()

  window.__CROWBAR__ = {
    mode: 'local',
    endpoint: (import.meta as { env?: { VITE_API_URL?: string } }).env?.VITE_API_URL ?? 'http://localhost:7457',
    on: <T = unknown>(event: string, handler: (payload: T) => void) => {
      if (!listeners.has(event)) listeners.set(event, new Set())
      listeners.get(event)!.add(handler as (payload: unknown) => void)
      return () => {
        listeners.get(event)?.delete(handler as (payload: unknown) => void)
      }
    },
    emit: (event: string, payload?: unknown) => {
      listeners.get(event)?.forEach(h => h(payload))
    },
  }
}
