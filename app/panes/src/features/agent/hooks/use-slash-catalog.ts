import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  getSlashCatalog,
  type SlashCatalog,
  type SlashCatalogItem,
} from '@/features/agent/api/agent-chat-reads'
import { ApiError } from '@/lib/errors/api-error'

const SLASH_DEBOUNCE_MS = 150

export function commandGroup(item: Partial<Pick<SlashCatalogItem, 'kind' | 'terminal'>>): number {
  if (item.kind === 'skill') return 2
  return item.terminal ? 1 : 0
}

type SlashCatalogState =
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

export function useSlashCatalog({ wsId, chatId, providerId, active }: SlashCatalogOptions) {
  const [state, setState] = useState<SlashCatalogState>({ state: 'closed' })
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState(0)
  const [query, setQuery] = useState('')
  const leadingRef = useRef(false)
  const providerRef = useRef(providerId)
  providerRef.current = providerId

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

  const cached = state.state === 'ready' || (state.state === 'error' && state.unavailable)
  useEffect(() => {
    if (!active || cached) return
    const controller = new AbortController()
    void probe(controller.signal)
    return () => controller.abort()
  }, [active, cached, probe])

  useEffect(() => {
    if (!open || !active) return
    const controller = new AbortController()
    const timer = window.setTimeout(() => void probe(controller.signal), SLASH_DEBOUNCE_MS)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [open, active, probe])

  const ranked = useMemo(() => {
    if (state.state !== 'ready') return []
    if (!query) return state.catalog.items
    const named = (item: SlashCatalogItem) =>
      item.insertText.replace(/^[/$]/, '').toLowerCase().startsWith(query)
    const mentions = (item: SlashCatalogItem) =>
      `${item.label}\n${item.description}\n${item.source}\n${item.insertText}`
        .toLowerCase()
        .includes(query)
    const all = state.catalog.items
    return [...all.filter(named), ...all.filter((item) => !named(item) && mentions(item))]
  }, [state, query])
  const items = useMemo(
    () => [...ranked].sort((a, b) => commandGroup(a) - commandGroup(b)),
    [ranked],
  )
  const total = state.state === 'ready' ? state.catalog.items.length : 0

  useEffect(() => {
    if (selected >= items.length) setSelected(Math.max(0, items.length - 1))
  }, [items.length, selected])

  const move = useCallback(
    (direction: 1 | -1) =>
      setSelected((current) => (current + direction + items.length) % items.length),
    [items.length],
  )

  const accept = useCallback((item: Pick<SlashCatalogItem, 'insertText'>) => {
    leadingRef.current = item.insertText.startsWith('/')
    setOpen(false)
    return item.insertText
  }, [])

  const highlighted = open ? items[selected] : undefined

  return {
    state,
    open,
    items,
    total,
    query,
    selected,
    highlighted,
    noteQuery,
    close,
    reset,
    move,
    accept,
  }
}
