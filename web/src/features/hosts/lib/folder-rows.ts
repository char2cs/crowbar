import type { BrowseEntry } from '@/features/hosts/lib/browse'

/** A directory's listing as far as it has come. */
export type Listing =
  | { status: 'loading' }
  | { status: 'loaded'; entries: BrowseEntry[] }
  | { status: 'failed'; error: unknown }

/** What an expanded folder shows in place of children it does not have yet. */
export type FolderNote =
  | { type: 'waiting' }
  | { type: 'loading' }
  | { type: 'empty' }
  | { type: 'failed'; error: unknown }
  | { type: 'more-files'; hidden: number }

export type FolderRow =
  | { kind: 'entry'; key: string; level: number; entry: BrowseEntry; expanded?: boolean }
  | { kind: 'note'; key: string; level: number; note: FolderNote }

// The daemon lists a directory whole. Every folder is shown — folders are what can be chosen —
// and past this many the inert files are cut.
const FILE_CAP = 500

// Only a directory's own children nest under it: an entry pointing back at an ancestor (or into
// another directory) would recurse forever or repeat a row, whatever the daemon answered.
export function childrenOf(dir: string, entries: readonly BrowseEntry[]): BrowseEntry[] {
  const prefix = dir.endsWith('/') ? dir : `${dir}/`
  return entries.filter(
    (e) =>
      e.path.length > prefix.length &&
      e.path.startsWith(prefix) &&
      !e.path.includes('/', prefix.length),
  )
}

function noteFor(dir: string, listing: Listing | undefined): FolderNote | null {
  if (listing === undefined) return { type: 'waiting' }
  if (listing.status === 'loading') return { type: 'loading' }
  if (listing.status === 'failed') return { type: 'failed', error: listing.error }
  return childrenOf(dir, listing.entries).length === 0 ? { type: 'empty' } : null
}

/** The visible rows under `parent`: its entries, and below each expanded folder its own
 *  rows or the note that stands in for them. */
export function folderRows(
  parent: string,
  entries: readonly BrowseEntry[],
  listings: Readonly<Record<string, Listing>>,
  expanded: ReadonlySet<string>,
  level = 1,
): FolderRow[] {
  const own = childrenOf(parent, entries)
  const folders = own.filter((e) => e.isDir)
  const files = own.filter((e) => !e.isDir)
  const rows: FolderRow[] = []
  for (const entry of [...folders, ...files.slice(0, FILE_CAP)]) {
    const open = expanded.has(entry.path)
    rows.push({
      kind: 'entry',
      key: entry.path,
      level,
      entry,
      expanded: entry.isDir ? open : undefined,
    })
    if (!entry.isDir || !open) continue
    const listing = listings[entry.path]
    const note = noteFor(entry.path, listing)
    if (note) rows.push({ kind: 'note', key: `${entry.path}\0note`, level: level + 1, note })
    else if (listing?.status === 'loaded') {
      rows.push(...folderRows(entry.path, listing.entries, listings, expanded, level + 1))
    }
  }
  if (files.length > FILE_CAP) {
    const note: FolderNote = { type: 'more-files', hidden: files.length - FILE_CAP }
    rows.push({ kind: 'note', key: `${parent}\0more`, level, note })
  }
  return rows
}

/** What a failed request says, for showing it. */
export function failureText(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

/** The words a note row shows. */
export function folderNoteText(note: FolderNote, hostName: string): string {
  switch (note.type) {
    case 'waiting':
      return `Waiting for ${hostName} to answer`
    case 'loading':
      return 'Loading…'
    case 'empty':
      return 'Empty folder'
    case 'failed':
      return `Can't open: ${failureText(note.error)}`
    case 'more-files':
      return `${note.hidden} more ${note.hidden === 1 ? 'file' : 'files'} not shown`
  }
}
