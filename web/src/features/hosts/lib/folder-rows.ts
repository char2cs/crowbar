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
  | { type: 'more'; hidden: number }

export type FolderRow =
  | { kind: 'entry'; key: string; level: number; entry: BrowseEntry; expanded?: boolean }
  | { kind: 'note'; key: string; level: number; note: FolderNote }

// The daemon lists a directory whole; past this many entries a listing is cut, folders kept first.
const LISTING_CAP = 500

function foldersFirst(entries: readonly BrowseEntry[]): BrowseEntry[] {
  return [...entries.filter((e) => e.isDir), ...entries.filter((e) => !e.isDir)]
}

function noteFor(listing: Listing | undefined): FolderNote | null {
  if (listing === undefined) return { type: 'waiting' }
  if (listing.status === 'loading') return { type: 'loading' }
  if (listing.status === 'failed') return { type: 'failed', error: listing.error }
  return listing.entries.length === 0 ? { type: 'empty' } : null
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
  const ordered = foldersFirst(entries)
  const rows: FolderRow[] = []
  for (const entry of ordered.slice(0, LISTING_CAP)) {
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
    const note = noteFor(listing)
    if (note) rows.push({ kind: 'note', key: `${entry.path}\0note`, level: level + 1, note })
    else if (listing?.status === 'loaded') {
      rows.push(...folderRows(entry.path, listing.entries, listings, expanded, level + 1))
    }
  }
  if (ordered.length > LISTING_CAP) {
    const note: FolderNote = { type: 'more', hidden: ordered.length - LISTING_CAP }
    rows.push({ kind: 'note', key: `${parent}\0more`, level, note })
  }
  return rows
}
