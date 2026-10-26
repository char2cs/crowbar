import { useEffect, useSyncExternalStore } from 'react'

// An icon is a small image; anything bigger is not drawn rather than shipped through a snapshot.
const MAX_BYTES = 256 * 1024

/** Each URL's data URI once read; null when it could not be (failed, or too big). */
const read = new Map<string, string | null>()
const listeners = new Set<() => void>()
let version = 0

function settle(url: string, data: string | null) {
  read.set(url, data)
  version += 1
  listeners.forEach((listener) => listener())
}

async function load(url: string) {
  read.set(url, null)
  try {
    const res = await fetch(url)
    const blob = res.ok ? await res.blob() : null
    if (!blob || blob.size > MAX_BYTES) return settle(url, null)
    const data = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.onerror = () => reject(reader.error ?? new Error('icon read failed'))
      reader.readAsDataURL(blob)
    })
    settle(url, data)
  } catch {
    settle(url, null)
  }
}

const subscribe = (listener: () => void) => {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/**
 * Icons as data URIs, for a page that cannot reach the daemon an icon URL points at (the
 * phone's chat list). Read once per URL, from this webview, which can.
 */
export function useIconDataUris(urls: readonly string[]): ReadonlyMap<string, string | null> {
  useSyncExternalStore(subscribe, () => version)
  const key = urls.join('\n')
  // react-doctor-disable-next-line no-fetch-in-effect -- each URL is fetched once into the module cache and settled by URL, so a rerun or unmount can neither race nor double-fire it; the app has no data-fetching layer.
  useEffect(() => {
    for (const url of key.split('\n')) if (url && !read.has(url)) void load(url)
  }, [key])
  return read
}
