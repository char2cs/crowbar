import { until } from './wait.mjs'

const BASE = 'http://crowbar.local'

/** fnv1a64 of the home string, byte-identical to the daemon's own socket derivation. */
export function socketPathFor(home, tmpdir) {
  let h = 0xcbf29ce484222325n
  for (const byte of new TextEncoder().encode(home)) {
    h ^= BigInt(byte)
    h = (h * 0x100000001b3n) & 0xffffffffffffffffn
  }
  return `${tmpdir.replace(/\/$/, '')}/crowbar-${h.toString(16)}.sock`
}

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
    waitHealthy(timeout = 60_000) {
      return until('the daemon to answer /v0/health', () => this.health(), {
        timeout,
      })
    },
  }
}
