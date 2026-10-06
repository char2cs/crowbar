import { writeFileSync } from 'node:fs'
import { until } from './wait.mjs'

// The debug build of the app embeds tauri-plugin-mcp-bridge, a WebSocket server
// on the first free port from 9223 (the same one the Tauri MCP uses). It speaks
// {id, command, args} -> {id, success, data}. macOS evaluates scripts natively
// and only for scripts without promises, so every page script is synchronous:
// anything that must wait is polled from here against a deadline.
const BASE_PORT = 9223
const PORT_SPAN = 100
// The bridge hands scripts containing any of these to a different, IPC-callback
// path that is unreliable on WKWebView; refuse them rather than flake.
const ASYNC_MARKERS = ['await ', 'async ', '.then(', 'Promise.', 'new Promise(']

function request(ws, command, args, timeout = 20_000) {
  const id = crypto.randomUUID()
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      ws.removeEventListener('message', onMessage)
      reject(new Error(`driver command ${command} timed out`))
    }, timeout)
    function onMessage(event) {
      let message
      try {
        message = JSON.parse(event.data)
      } catch {
        return
      }
      if (message.id !== id) return
      clearTimeout(timer)
      ws.removeEventListener('message', onMessage)
      if (message.success === false) reject(new Error(message.error ?? `${command} failed`))
      else resolve(message.data)
    }
    ws.addEventListener('message', onMessage)
    ws.send(JSON.stringify({ id, command, args }))
  })
}

function openSocket(port) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(`ws://127.0.0.1:${port}`)
    const timer = setTimeout(() => {
      ws.close()
      reject(new Error('connect timeout'))
    }, 1500)
    ws.addEventListener('open', () => {
      clearTimeout(timer)
      resolve(ws)
    })
    ws.addEventListener('error', () => {
      clearTimeout(timer)
      reject(new Error('connect failed'))
    })
  })
}

export class Fatal extends Error {
  name = 'Fatal'
}

export class Driver {
  constructor(ws, port) {
    this.ws = ws
    this.port = port
  }

  /**
   * Finds the bridge of the app whose webview is served from `devPort`. Several
   * dev apps can run on one machine (one bridge each); the origin tells ours apart.
   */
  static async attach(devPort, { timeout = 120_000, failed = () => null } = {}) {
    return until(
      `the app on origin :${devPort} to expose its driver`,
      async () => {
        const reason = failed()
        if (reason) throw new Fatal(reason)
        for (let port = BASE_PORT; port < BASE_PORT + PORT_SPAN; port++) {
          let ws
          try {
            ws = await openSocket(port)
          } catch {
            continue
          }
          try {
            const origin = await request(ws, 'execute_js', { script: 'return location.port' }, 3000)
            if (String(origin) === String(devPort)) return new Driver(ws, port)
          } catch {
            // not ours, or not loaded yet
          }
          ws.close()
        }
        return null
      },
      { timeout },
    )
  }

  close() {
    this.ws.close()
  }

  /** Runs `fn(arg)` in the webview and returns its JSON-serialisable result. */
  async eval(fn, arg) {
    const source = `return (${fn.toString()})(${JSON.stringify(arg ?? null)})`
    for (const marker of ASYNC_MARKERS) {
      if (source.includes(marker))
        throw new Error(`page script must be synchronous (contains "${marker}")`)
    }
    return request(this.ws, 'execute_js', { script: source })
  }

  /** Polls a page-side predicate until it returns something truthy. */
  until(describe, fn, arg, options) {
    return until(describe, () => this.eval(fn, arg), options)
  }

  /** Emits a Tauri event into the app, as the native menu does. */
  emit(eventName, payload = null) {
    return request(this.ws, 'invoke_tauri', {
      command: 'plugin:mcp-bridge|emit_event',
      args: { eventName, payload },
    })
  }

  async screenshot(file) {
    const shot = await request(this.ws, 'capture_native_screenshot', { format: 'png' }, 30_000)
    writeFileSync(file, Buffer.from(String(shot.dataUrl).split(',')[1], 'base64'))
  }
}
