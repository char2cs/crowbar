const created = new Map<string, Promise<unknown>>()

export function registerWebview(id: string, promise: Promise<unknown>): void {
  created.set(id, promise)
}

export function unregisterWebview(id: string): Promise<unknown> | undefined {
  const promise = created.get(id)
  created.delete(id)
  return promise
}

export function webviewCreated(id: string): Promise<unknown> | undefined {
  return created.get(id)
}
