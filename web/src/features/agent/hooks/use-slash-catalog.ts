import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  getSlashCatalog,
  type SlashCatalog,
  type SlashCatalogItem,
} from '@/features/agent/api/agent-chat-reads'
import { ApiError } from '@/lib/errors/api-error'

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
}

// The slash picker's catalogue, fetched as soon as the chat is active since the probe can take
// seconds. Opening the picker refreshes it silently, never blanking the cache, so there is no
// loading state; catalogues are partial by declaration, so an empty one is not an error.
export function useSlashCatalog({ wsId, chatId, providerId, active }: SlashCatalogOptions) {
  const [state, setState] = useState<SlashCatalogState>({ state: 'closed' })
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState(0)
  const [query, setQuery] = useState('')
  const leadingRef = useRef(false)
  const providerRef = useRef(providerId)
  providerRef.current = providerId

  /** Told the box's slash query on every edit, so a leading `/` opens the picker
   *  exactly once (retyping it reopens after a dismissal). Text with no slash on
   *  either side of the edit sets nothing, so plain typing renders nothing here. */
  const noteQuery = useCallback((slashQuery: string | null) => {
    const leading = slashQuery !== null
    const was = leadingRef.current
    leadingRef.current = leading
    if (!leading && !was) return
    setQuery((slashQuery ?? '').trim().toLowerCase())
    if (leading && !was) {
      setOpen(true)
      setSelected(0)
    } else if (!leading) {
      setOpen(false)
    }
  }, [])

  const close = useCallback(() => setOpen(false), [])

  const reset = useCallback(() => {
    leadingRef.current = false
    setOpen(false)
  }, [])

  // Another chat or provider means another CLI's skills. AgentChatView's key remounts on chatId
  // but not on providerId alone, so a key cannot replace this reset.
  useEffect(() => {
    // react-doctor-disable-next-line react-doctor/no-adjust-state-on-prop-change -- a provider switch keeps the key (see above), so only this reset closes the old CLI's menu.
    setOpen(false)
    // react-doctor-disable-next-line react-doctor/no-adjust-state-on-prop-change -- same reset: the old CLI's catalog must not answer the new one's query.
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

  // Chat initiation: fetched once, well before the first `/`, and not again on a
  // later show — the cached answer stands (a catalogue, or the daemon's 422 that
  // there is none), and opening the picker refreshes it.
  const cached = state.state === 'ready' || (state.state === 'error' && state.unavailable)
  useEffect(() => {
    if (!active || cached) return
    const controller = new AbortController()
    void probe(controller.signal)
    return () => controller.abort()
  }, [active, cached, probe])

  // Reopening asks again, still silently — whatever is cached rides until the refresh lands,
  // debounced so a `/` typed and immediately deleted never fires a probe at all.
  useEffect(() => {
    if (!open || !active) return
    const controller = new AbortController()
    const timer = window.setTimeout(() => void probe(controller.signal), SLASH_DEBOUNCE_MS)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [open, active, probe])

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
  const accept = useCallback((item: Pick<SlashCatalogItem, 'insertText'>) => {
    leadingRef.current = item.insertText.startsWith('/')
    setOpen(false)
    return item.insertText
  }, [])

  // The one thing Enter and Tab both need: is there a completion to accept? An
  // open picker is not enough — it is open while the probe is still running, and
  // open over an empty result for every command the probe cannot see.
  const highlighted = open ? items[selected] : undefined

  return { state, open, items, selected, highlighted, noteQuery, close, reset, move, accept }
}
