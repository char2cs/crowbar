import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  getSlashCatalog,
  type SlashCatalog,
  type SlashCatalogItem,
} from '@/features/agent/api/agent-api'
import { ApiError } from '@/lib/api'

const SLASH_DEBOUNCE_MS = 150

export type SlashCatalogState =
  | { state: 'closed' }
  | { state: 'ready'; catalog: SlashCatalog }
  | { state: 'error'; error: Error; unavailable: boolean }

function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

export interface SlashCatalogOptions {
  wsId: string
  chatId: string
  providerId: string
  active: boolean
  /** The composer's current text. Only its leading `/` and query tail matter. */
  draft: string
}

/**
 * The slash picker's catalogue.
 *
 * PRESAVED, not lazy: the probe can shell out to the CLI and take seconds, so
 * it fetches once as soon as the chat is active — before anybody has typed a
 * `/` — and caches whatever comes back. Opening the picker (the one moment the
 * user actually goes into it) asks again in the background, in case skills
 * changed since, but the refresh is silent: it never blanks what is already
 * cached, so the picker has no loading state to show and never shows a
 * spinner. The catalogue is incomplete by declaration on every provider
 * Crowbar ships (see `SlashCatalog.completeness`), which is why nothing here
 * treats an empty result as an error either.
 */
export function useSlashCatalog({ wsId, chatId, providerId, active, draft }: SlashCatalogOptions) {
  const [state, setState] = useState<SlashCatalogState>({ state: 'closed' })
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState(0)
  const leadingRef = useRef(false)
  const providerRef = useRef(providerId)
  providerRef.current = providerId

  /** Told about every draft change, so a leading `/` can open the picker exactly
   *  once — retyping the slash is what reopens it after an explicit dismissal. */
  const noteDraft = useCallback((value: string) => {
    const leading = value.startsWith('/')
    if (leading && !leadingRef.current) {
      setOpen(true)
      setSelected(0)
    } else if (!leading) {
      setOpen(false)
    }
    leadingRef.current = leading
  }, [])

  const close = useCallback(() => setOpen(false), [])

  const reset = useCallback(() => {
    leadingRef.current = false
    setOpen(false)
  }, [])

  // A chat/provider switch invalidates whatever is cached — those are a
  // different CLI's skills, never shown stale under a menu that looks like
  // it belongs to the one now running.
  useEffect(() => {
    setOpen(false)
    setState({ state: 'closed' })
  }, [wsId, chatId, providerId])

  const probe = useCallback(
    (signal: AbortSignal) => {
      const providerAtRequest = providerId
      return getSlashCatalog(wsId, chatId, signal)
        .then((catalog) => {
          if (
            signal.aborted ||
            providerAtRequest !== providerRef.current ||
            catalog.providerId !== providerAtRequest
          )
            return
          setState({ state: 'ready', catalog })
        })
        .catch((error: unknown) => {
          if (signal.aborted || isAbort(error)) return
          setState({
            state: 'error',
            error: error instanceof Error ? error : new Error(String(error)),
            unavailable: error instanceof ApiError && error.status === 422,
          })
        })
    },
    [wsId, chatId, providerId],
  )

  // Chat initiation: fetched once, well before the first `/`.
  useEffect(() => {
    if (!active) return
    const controller = new AbortController()
    void probe(controller.signal)
    return () => controller.abort()
  }, [active, probe])

  // Reopening asks again, still silently — whatever is cached rides until the
  // refresh lands, debounced so a `/` typed and immediately deleted never
  // fires a probe at all.
  useEffect(() => {
    if (!open || !active) return
    const controller = new AbortController()
    const timer = window.setTimeout(() => void probe(controller.signal), SLASH_DEBOUNCE_MS)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [open, active, probe])

  const query = draft.slice(1).trim().toLowerCase()
  const items = useMemo(() => {
    if (state.state !== 'ready') return []
    if (!query) return state.catalog.items
    return state.catalog.items.filter((item) =>
      `${item.label}\n${item.description}\n${item.source}\n${item.insertText}`
        .toLowerCase()
        .includes(query),
    )
  }, [state, query])

  useEffect(() => {
    if (selected >= items.length) setSelected(Math.max(0, items.length - 1))
  }, [items.length, selected])

  const move = useCallback(
    (direction: 1 | -1) =>
      setSelected((current) => (current + direction + items.length) % items.length),
    [items.length],
  )

  /** Accept an item: the caller writes `insertText` into the composer. */
  const accept = useCallback((item: SlashCatalogItem) => {
    leadingRef.current = item.insertText.startsWith('/')
    setOpen(false)
    return item.insertText
  }, [])

  // The one thing Enter and Tab both need: is there a completion to accept? An
  // open picker is not enough — it is open while the probe is still running, and
  // open over an empty result for every command the probe cannot see.
  const highlighted = open ? items[selected] : undefined

  return { state, open, items, selected, highlighted, noteDraft, close, reset, move, accept }
}
