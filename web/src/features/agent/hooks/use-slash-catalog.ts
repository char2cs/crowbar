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
  | { state: 'loading' }
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
 * It opens on a LEADING slash and probes the provider once, debounced — the
 * probe can shell out to the CLI and take seconds, so it is never re-run per
 * keystroke; the query filters what came back. The catalogue is incomplete by
 * declaration on every provider Crowbar ships (see `SlashCatalog.completeness`),
 * which is why nothing here treats an empty result as an error.
 */
export function useSlashCatalog({ wsId, chatId, providerId, active, draft }: SlashCatalogOptions) {
  const [state, setState] = useState<SlashCatalogState>({ state: 'closed' })
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState(0)
  const leadingRef = useRef(false)
  const providerRef = useRef(providerId)

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

  // Provider/view changes close and abort the one-open-menu catalog. A leading
  // slash must be deleted/retyped to explicitly open a fresh provider probe.
  useEffect(() => {
    if (providerRef.current !== providerId || !active) {
      providerRef.current = providerId
      setOpen(false)
      setState({ state: 'closed' })
    }
  }, [providerId, active])

  useEffect(() => {
    if (!open || !active) {
      setState({ state: 'closed' })
      return
    }
    const providerAtRequest = providerId
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      setState({ state: 'loading' })
      void getSlashCatalog(wsId, chatId, controller.signal)
        .then((catalog) => {
          if (
            controller.signal.aborted ||
            providerAtRequest !== providerRef.current ||
            catalog.providerId !== providerAtRequest
          )
            return
          setState({ state: 'ready', catalog })
          setSelected(0)
        })
        .catch((error: unknown) => {
          if (controller.signal.aborted || isAbort(error)) return
          setState({
            state: 'error',
            error: error instanceof Error ? error : new Error(String(error)),
            unavailable: error instanceof ApiError && error.status === 422,
          })
        })
    }, SLASH_DEBOUNCE_MS)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [open, active, wsId, chatId, providerId])

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
