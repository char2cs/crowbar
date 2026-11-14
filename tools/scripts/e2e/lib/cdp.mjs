import { writeFileSync } from 'node:fs'
import { until } from './wait.mjs'

// The browser target: raw Chrome DevTools Protocol over a WebSocket to a Chrome
// this suite launched itself. Same surface as the Tauri Driver, so scenarios do
// not care which they run on.
export class CdpDriver {
  constructor(ws, debugPort, targetId) {
    this.ws = ws
    this.debugPort = debugPort
    this.targetId = targetId
    this.next = 1
    this.pending = new Map()
    ws.addEventListener('message', (event) => {
      const message = JSON.parse(event.data)
      const waiter = this.pending.get(message.id)
      if (!waiter) return
      this.pending.delete(message.id)
      if (message.error) waiter.reject(new Error(message.error.message))
      else waiter.resolve(message.result)
    })
  }

  static async attach(debugPort, url) {
    const target = await until('Chrome to open the page', async () => {
      const res = await fetch(`http://127.0.0.1:${debugPort}/json/new?${encodeURIComponent(url)}`, {
        method: 'PUT',
      })
      return res.ok ? res.json() : null
    })
    const ws = new WebSocket(target.webSocketDebuggerUrl)
    await new Promise((resolve, reject) => {
      ws.addEventListener('open', resolve)
      ws.addEventListener('error', () => reject(new Error('CDP connect failed')))
    })
    return new CdpDriver(ws, debugPort, target.id)
  }

  send(method, params = {}) {
    const id = this.next++
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      this.ws.send(JSON.stringify({ id, method, params }))
    })
  }

  close() {
    this.ws.close()
  }

  /** Closes this tab; leaving first lets the browser commit its pending localStorage writes. */
  async closeTab() {
    await this.send('Page.navigate', { url: 'about:blank' })
    this.ws.close()
    await fetch(`http://127.0.0.1:${this.debugPort}/json/close/${this.targetId}`)
  }

  async eval(fn, arg) {
    const expression = `(${fn.toString()})(${JSON.stringify(arg ?? null)})`
    const res = await this.send('Runtime.evaluate', {
      expression,
      returnByValue: true,
    })
    if (res.exceptionDetails) {
      throw new Error(res.exceptionDetails.exception?.description ?? res.exceptionDetails.text)
    }
    return res.result.value ?? null
  }

  until(describe, fn, arg, options) {
    return until(describe, () => this.eval(fn, arg), options)
  }

  emit() {
    throw new Error('native menu events exist only on the Tauri target')
  }

  async screenshot(file) {
    const { data } = await this.send('Page.captureScreenshot', {
      format: 'png',
    })
    writeFileSync(file, Buffer.from(data, 'base64'))
  }

  async reload() {
    await this.send('Page.reload')
  }
}
