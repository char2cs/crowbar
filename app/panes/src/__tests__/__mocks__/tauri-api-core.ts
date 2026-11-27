export async function invoke(_cmd: string, _args?: Record<string, unknown>): Promise<void> {}

export function convertFileSrc(filePath: string, protocol = 'asset'): string {
  return `${protocol}://localhost/${encodeURIComponent(filePath)}`
}

export class Channel<T> {
  onmessage: (message: T) => void = () => undefined
}
