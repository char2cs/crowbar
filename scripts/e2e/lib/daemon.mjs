import { Fatal } from './driver.mjs'
import { until } from './wait.mjs'

const BASE = 'http://crowbar.local'

/** A client for one daemon over its unix socket. Bun's fetch takes the socket as an option. */
export function daemonClient(socket) {
  async function call(method, path, body) {
    const tcp = socket.startsWith('http')
    const res = await fetch((tcp ? socket : BASE) + path, {
      method,
      ...(tcp ? {} : { unix: socket }),
      headers: body === undefined ? undefined : { 'content-type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const text = await res.text()
    let json = null
    try {
      json = text ? JSON.parse(text) : null
    } catch {
      // not JSON: surfaced through `text`
    }
    return { status: res.status, ok: res.ok, json, text }
  }
  async function data(method, path, body) {
    const res = await call(method, path, body)
    if (!res.ok) throw new Error(`${method} ${path} -> ${res.status}: ${res.text.slice(0, 300)}`)
    return res.json?.data
  }
  return {
    socket,
    call,
    get: (path) => data('GET', path),
    post: (path, body) => data('POST', path, body ?? {}),
    patch: (path, body) => data('PATCH', path, body),
    async health() {
      try {
        const res = await call('GET', '/v0/health')
        return res.ok ? res.json : null
      } catch {
        return null
      }
    },
    /**
     * Waits until the daemon serves its API: a migrating daemon answers health
     * too. A halted one never will, so it fails at once with the reason.
     */
    waitHealthy(timeout = 60_000) {
      return until(
        'the daemon to report ok on /v0/health',
        async () => {
          const health = await this.health()
          const status = health?.data?.status
          if (status === 'halted') {
            throw new Fatal(
              `the daemon halted: ${health.data.detail?.message ?? 'no reason given'}`,
            )
          }
          return status === 'ok' ? health : null
        },
        { timeout },
      )
    },
  }
}
