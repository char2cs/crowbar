import { apiFetchRaw } from '@/lib/api/fetch'

/** The webview reports no progress for a request body, so a big file goes as acknowledged pieces:
 *  progress is the bytes the daemon says it holds. */
export const CHUNK_BYTES = 1024 * 1024

interface ChunkedOptions {
  onProgress?: (loaded: number, total: number) => void
  signal?: AbortSignal
}

const heldBytes = async (response: Response) =>
  ((await response.json()) as { data: { received: number } }).data.received

/** Sends `file` to the daemon under `uploads` as chunks and answers the completion response. A
 *  failed upload keeps its staged bytes, so sending the same `id` again resumes where the daemon
 *  stopped; an aborted one deletes them. */
export async function uploadInChunks(
  uploads: string,
  file: File,
  id: string,
  { onProgress, signal }: ChunkedOptions,
): Promise<Response> {
  const upload = `${uploads}/${encodeURIComponent(id)}`
  const json = (path: string, method: string, body: object) =>
    apiFetchRaw(path, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal,
    })
  try {
    let held = await heldBytes(
      await json(uploads, 'POST', { id, fileName: file.name, size: file.size }),
    )
    onProgress?.(held, file.size)
    while (held < file.size) {
      const chunk = new Uint8Array(await file.slice(held, held + CHUNK_BYTES).arrayBuffer())
      held = await heldBytes(
        await apiFetchRaw(`${upload}?offset=${held}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/octet-stream' },
          body: chunk,
          signal,
        }),
      )
      onProgress?.(held, file.size)
    }
    return await json(`${upload}/complete`, 'POST', { fileName: file.name, size: file.size })
  } catch (err) {
    if (signal?.aborted) await discardStaged(upload)
    throw err
  }
}

// Best effort: the daemon also reaps a stale upload when the chat's next one begins.
async function discardStaged(upload: string): Promise<void> {
  try {
    await apiFetchRaw(upload, { method: 'DELETE' })
  } catch (err) {
    console.warn('could not discard a cancelled upload', err)
  }
}
