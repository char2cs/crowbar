import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { fetchFileTree } from '@/features/files/lib/file-tree-api'
import type { AppFile } from '@/features/file-system/types/app'

/** One workspace entry the @ picker offers; `insertText` replaces the @ path typed so far. */
export interface MentionItem {
  id: string
  insertText: string
  description: string
}

const MAX_ITEMS = 50

/** The folder an @ path is in, and what is typed of the name after it. */
function splitQuery(query: string): { dir: string; name: string } {
  const slash = query.lastIndexOf('/')
  return slash < 0
    ? { dir: '', name: query }
    : { dir: query.slice(0, slash), name: query.slice(slash + 1) }
}

const itemOf = (file: AppFile): MentionItem => ({
  id: file.path,
  // A folder stays a path to go on typing; a file ends the mention.
  insertText: file.isDir ? `@${file.path}/` : `@${file.path} `,
  description: file.isDir ? 'Folder' : '',
})

/** The composer's @ file picker. The tree endpoint lists one folder per call, so it
 *  completes a path a folder at a time, as a shell does, rather than searching. */
export function useFileMention(wsId: string) {
  const [query, setQuery] = useState<string | null>(null)
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState(0)
  const [listing, setListing] = useState<{ dir: string; files: AppFile[] } | null>(null)
  const { dir, name } = splitQuery(query ?? '')

  const typingRef = useRef(false)

  /** Told the @ path before the caret on every edit; a new @ opens the picker. */
  const noteQuery = useCallback((next: string | null) => {
    const was = typingRef.current
    typingRef.current = next !== null
    if (next === null && !was) return
    setQuery(next)
    if (next !== null && !was) {
      setOpen(true)
      setSelected(0)
    } else if (next === null) setOpen(false)
  }, [])

  useEffect(() => {
    if (!open) return
    let current = true
    fetchFileTree(wsId, dir || undefined)
      .then((files) => {
        if (current) setListing({ dir, files })
      })
      .catch(() => {
        if (current) setListing({ dir, files: [] })
      })
    return () => {
      current = false
    }
  }, [open, wsId, dir])

  const items = useMemo(() => {
    if (!open || listing?.dir !== dir) return []
    const typed = name.toLowerCase()
    return listing.files
      .filter((file) => file.name.toLowerCase().includes(typed))
      .slice(0, MAX_ITEMS)
      .map(itemOf)
  }, [open, listing, dir, name])

  const shown = Math.min(selected, Math.max(0, items.length - 1))
  const move = useCallback(
    (direction: 1 | -1) =>
      setSelected((current) => (current + direction + items.length) % Math.max(1, items.length)),
    [items.length],
  )
  const close = useCallback(() => setOpen(false), [])
  /** Accept an item: the caller puts `insertText` in place of the @ path. */
  const accept = useCallback((item: MentionItem) => {
    setOpen(item.insertText.endsWith('/'))
    setSelected(0)
    return item.insertText
  }, [])

  const highlighted = open ? items[shown] : undefined
  return { open, items, selected: shown, highlighted, noteQuery, close, move, accept }
}
