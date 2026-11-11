/** A `fetch` that sends through XMLHttpRequest so the request body's upload can be measured;
 *  fetch has no upload progress. Answers a Response, and rejects like fetch does: a TypeError for
 *  no answer, an AbortError when `init.signal` fires. */
export function progressFetch(onProgress: (loaded: number, total: number) => void): typeof fetch {
  return (input, init) =>
    new Promise<Response>((resolve, reject) => {
      const signal = init?.signal
      if (signal?.aborted) return reject(new DOMException('Aborted', 'AbortError'))
      const xhr = new XMLHttpRequest()
      xhr.open(init?.method ?? 'GET', String(input))
      xhr.responseType = 'arraybuffer'
      new Headers(init?.headers).forEach((value, name) => xhr.setRequestHeader(name, value))
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) onProgress(event.loaded, event.total)
      }
      xhr.onload = () => {
        const headers = new Headers()
        for (const line of xhr.getAllResponseHeaders().trim().split(/\r?\n/)) {
          const at = line.indexOf(':')
          if (at > 0) headers.append(line.slice(0, at).trim(), line.slice(at + 1).trim())
        }
        const empty = [204, 205, 304].includes(xhr.status)
        resolve(
          new Response(empty ? null : (xhr.response as ArrayBuffer), {
            status: xhr.status,
            statusText: xhr.statusText,
            headers,
          }),
        )
      }
      xhr.onerror = () => reject(new TypeError('Network request failed'))
      xhr.onabort = () => reject(new DOMException('Aborted', 'AbortError'))
      signal?.addEventListener('abort', () => xhr.abort(), { once: true })
      xhr.send((init?.body ?? null) as XMLHttpRequestBodyInit | null)
    })
}
