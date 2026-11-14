/** Tab id → settles when Rust has that tab's webview. The one record of whether it exists. */
const created = new Map<string, Promise<unknown>>()

/** Called by whoever creates a tab's webview. */
export function registerWebview(id: string, promise: Promise<unknown>): void {
  created.set(id, promise)
}

/** Forgets the tab's webview; returns when it exists, or undefined if it never did. */
export function unregisterWebview(id: string): Promise<unknown> | undefined {
  const promise = created.get(id)
  created.delete(id)
  return promise
}

/** When the tab's webview exists, or undefined while it has none (never shown). */
export function webviewCreated(id: string): Promise<unknown> | undefined {
  return created.get(id)
}
