interface Entry {
  /** Settles when Rust has the webview. */
  created: Promise<unknown>
  visible: boolean
}

const entries = new Map<string, Entry>()
const waiters = new Map<string, Set<() => void>>()

function wake(id: string): void {
  const pending = waiters.get(id)
  if (!pending) return
  waiters.delete(id)
  pending.forEach((fn) => fn())
}

function until(id: string, signal: AbortSignal, ready: () => boolean | Promise<boolean>) {
  return new Promise<void>((resolve, reject) => {
    const fail = () => {
      waiters.get(id)?.delete(attempt)
      reject(signal.reason as Error)
    }
    const attempt = () => {
      if (signal.aborted) return fail()
      Promise.resolve(ready()).then((ok) => {
        if (ok) return resolve()
        const set = waiters.get(id) ?? new Set<() => void>()
        waiters.set(id, set.add(attempt))
      }, reject)
    }
    signal.addEventListener('abort', fail, { once: true })
    attempt()
  })
}

/** Called by the pane that creates a tab's webview. */
export function registerWebview(id: string, created: Promise<unknown>): void {
  entries.set(id, { created, visible: false })
  wake(id)
}

/** Called once Rust has acknowledged showing or hiding the webview. */
export function setWebviewVisible(id: string, visible: boolean): void {
  const entry = entries.get(id)
  if (!entry) return
  entry.visible = visible
  wake(id)
}

/** Forgets the tab's webview; returns when it exists, or undefined if it never did. */
export function unregisterWebview(id: string): Promise<unknown> | undefined {
  const created = entries.get(id)?.created
  entries.delete(id)
  return created
}

/** When the tab's webview exists, or undefined while it has none (never shown). */
export function webviewCreated(id: string): Promise<unknown> | undefined {
  return entries.get(id)?.created
}

/** Resolves once the tab's webview exists. */
export function whenWebviewLive(id: string, signal: AbortSignal): Promise<void> {
  return until(id, signal, async () => {
    const entry = entries.get(id)
    if (!entry) return false
    await entry.created
    return true
  })
}

/** Resolves once the tab's webview is on screen. */
export function whenWebviewVisible(id: string, signal: AbortSignal): Promise<void> {
  return until(id, signal, () => entries.get(id)?.visible === true)
}
