interface CrowbarConfig {
  api: string
  events: string
}

const cfg: CrowbarConfig | null = (window as any).__CROWBAR__ ?? null

export const apiBase = cfg?.api ?? ''
export const eventUrl = cfg?.events ?? '/api/events'

export function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  return fetch(`${apiBase}${path}`, init)
}

export function createEventSource(): EventSource {
  return new EventSource(eventUrl)
}
