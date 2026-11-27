interface CrowbarBridge {
  mode: 'local' | 'remote'
  endpoint: string
  on: <T = unknown>(event: string, handler: (payload: T) => void) => () => void
  emit: (event: string, payload?: unknown) => void
}

declare global {
  interface Window {
    __CROWBAR__: CrowbarBridge
  }
}

export type {}
