interface CrowbarBridge {
  mode: 'local' | 'remote'
  /** Base URL for API calls. 'crowbar://api' locally, remote URL in production. */
  endpoint: string
  /** Subscribe to a daemon event. Returns an unlisten function. */
  on: <T = unknown>(event: string, handler: (payload: T) => void) => () => void
  /** Emit an event toward the daemon. */
  emit: (event: string, payload?: unknown) => void
}

declare global {
  interface Window {
    __CROWBAR__: CrowbarBridge
  }
}

export type {}
