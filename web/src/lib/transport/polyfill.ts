import './types'

// The native shell may inject a partial `window.__CROWBAR__` (just `api`/`endpoint`) without the
// event bridge, and an all-or-nothing guard would leave `on` undefined and crash
// connectDaemonEvents at startup — so ensure the object exists and backfill missing methods.
if (typeof window !== 'undefined') {
  const existing = (window.__CROWBAR__ ?? {}) as Partial<Window['__CROWBAR__']>
  const listeners = new Map<string, Set<(payload: unknown) => void>>()

  const polyfilled: Window['__CROWBAR__'] = {
    mode: existing.mode ?? 'local',
    endpoint: existing.endpoint ?? import.meta.env.VITE_API_URL ?? 'http://localhost:7457',
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
