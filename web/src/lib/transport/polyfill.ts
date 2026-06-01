import './types'

// The native Crowbar shell may inject a partial `window.__CROWBAR__` (e.g. just
// `api`/`endpoint`) before the event bridge (`on`/`emit`) is implemented — those
// are still FUTURE on the native side. An all-or-nothing guard would leave `on`
// undefined and crash `connectDaemonEvents` at startup. Instead, ensure the
// object exists and backfill any missing methods so the bridge is always whole.
if (typeof window !== 'undefined') {
  const existing = (window.__CROWBAR__ ?? {}) as Partial<Window['__CROWBAR__']>
  const listeners = new Map<string, Set<(payload: unknown) => void>>()

  const polyfilled: Window['__CROWBAR__'] = {
    mode: existing.mode ?? 'local',
    endpoint:
      existing.endpoint ??
      (import.meta as { env?: { VITE_API_URL?: string } }).env?.VITE_API_URL ??
      'http://localhost:7457',
    on:
      existing.on ??
      (<T = unknown>(event: string, handler: (payload: T) => void) => {
        if (!listeners.has(event)) listeners.set(event, new Set())
        listeners.get(event)!.add(handler as (payload: unknown) => void)
        return () => {
          listeners.get(event)?.delete(handler as (payload: unknown) => void)
        }
      }),
    emit:
      existing.emit ??
      ((event: string, payload?: unknown) => {
        listeners.get(event)?.forEach((h) => h(payload))
      }),
  }

  // Preserve any extra native-injected properties (e.g. `api`) by merging.
  window.__CROWBAR__ = Object.assign(existing, polyfilled)
}
