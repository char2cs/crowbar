import type { FileEntry } from '@/features/file-system/types/app'

export interface VisibleFileTreeRow {
  file: FileEntry
  depth: number
  isExpanded: boolean
  displayName?: string
}

export interface BuildVisibleFileTreeRowsOptions {
  compactFolders?: boolean
}

export interface FilterFileTreeForSearchResult {
  files: FileEntry[]
  expandedPaths: Set<string>
  matchedPaths: Set<string>
  orderedMatchedPaths: string[]
  matchCount: number
}

export interface FileTreeSearchHit {
  path: string
}

function getCompactFolderChild(item: FileEntry): FileEntry | null {
  if (!item.isDir || item.isEditing || item.isRenaming || item.isNewItem || !item.children) {
    return null
  }

  if (item.children.length !== 1) {
    return null
  }

  const child = item.children[0]
  if (!child.isDir || child.isEditing || child.isRenaming || child.isNewItem) {
    return null
  }

  return child
}

export function buildVisibleFileTreeRows(
  files: FileEntry[],
  expandedPaths: ReadonlySet<string>,
  options: BuildVisibleFileTreeRowsOptions = {},
): VisibleFileTreeRow[] {
  const rows: VisibleFileTreeRow[] = []
  const compactFolders = options.compactFolders === true

  const walk = (items: FileEntry[], depth: number) => {
    for (const item of items) {
      let rowFile = item
      const displayNameParts = [item.name]

      if (compactFolders) {
        while (expandedPaths.has(rowFile.path)) {
          const child = getCompactFolderChild(rowFile)
          if (!child) break

          rowFile = child
          displayNameParts.push(child.name)
        }
      }

      // A brand-new inline-edit placeholder (isNewItem) must never count as an
      // expanded directory — its path can be '' (the root), which would otherwise
      // load the workspace root as its children and duplicate the whole tree.
      const isExpanded = !!(rowFile.isDir && !rowFile.isNewItem && expandedPaths.has(rowFile.path))
      rows.push({
        file: rowFile,
        depth,
        isExpanded,
        displayName: displayNameParts.length > 1 ? displayNameParts.join('/') : undefined,
      })

      if (rowFile.isDir && isExpanded && rowFile.children) {
        walk(rowFile.children, depth + 1)
      }
    }
  }

  walk(files, 0)
  return rows
}

function normalizeSearchPath(path: string): string {
  const normalized = path.replace(/\\/g, '/')
  if (normalized === '/') return normalized
  return normalized.replace(/\/+$/g, '')
}

/**
 * Compute search hits for the file-tree filter: every loaded node whose NAME
 * contains `query` (case-insensitive substring). Matches both files and
 * directories; pass the result to filterFileTreeForFffHits to prune the tree to
 * the matches and their ancestors. Skips in-progress inline-edit placeholders.
 * (Only loaded levels are searched — the tree is lazy, so unexpanded directories
 * aren't traversed.)
 */
export function computeFileTreeSearchHits(files: FileEntry[], query: string): FileTreeSearchHit[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  const hits: FileTreeSearchHit[] = []
  const walk = (items: FileEntry[]): void => {
    for (const item of items) {
      if (item.isNewItem || item.isEditing) continue
      if (item.name.toLowerCase().includes(q)) hits.push({ path: item.path })
      if (item.children) walk(item.children)
    }
  }
  walk(files)
  return hits
}

export function filterFileTreeForFffHits(
  files: FileEntry[],
  hits: readonly FileTreeSearchHit[],
): FilterFileTreeForSearchResult {
  const expandedPaths = new Set<string>()
  const matchedPaths = new Set<string>()
  const hitPaths = hits.map((hit) => normalizeSearchPath(hit.path))
  const hitPathSet = new Set(hitPaths)
  const matchedTreePathByHitPath = new Map<string, string>()

  if (hitPathSet.size === 0) {
    return {
      files: [],
      expandedPaths,
      matchedPaths,
      orderedMatchedPaths: [],
      matchCount: 0,
    }
  }

  const walk = (items: FileEntry[]): FileEntry[] =>
    items.flatMap((item) => {
      const matchingChildren = item.children ? walk(item.children) : []
      const normalizedPath = normalizeSearchPath(item.path)
      const isMatch = hitPathSet.has(normalizedPath)

      if (!isMatch && matchingChildren.length === 0) {
        return []
      }

      if (isMatch) {
        matchedPaths.add(item.path)
        matchedTreePathByHitPath.set(normalizedPath, item.path)
      }

      if (item.isDir && matchingChildren.length > 0) {
        expandedPaths.add(item.path)
      }

      return [
        {
          ...item,
          children: matchingChildren.length > 0 ? matchingChildren : item.children,
        },
      ]
    })

  const filteredFiles = walk(files)
  const orderedMatchedPaths: string[] = []
  const seenOrderedPaths = new Set<string>()

  for (const hitPath of hitPaths) {
    const treePath = matchedTreePathByHitPath.get(hitPath)
    if (!treePath || seenOrderedPaths.has(treePath)) continue
    seenOrderedPaths.add(treePath)
    orderedMatchedPaths.push(treePath)
  }

  return {
    files: filteredFiles,
    expandedPaths,
    matchedPaths,
    orderedMatchedPaths,
    matchCount: matchedPaths.size,
  }
}

/** The shape of a TanStack `VirtualItem` this needs — kept minimal so this
 *  file doesn't have to import `@tanstack/react-virtual` just for a type. */
export interface VirtualRowExtent {
  index: number
  start: number
  size: number
}

/**
 * Which virtualized row is genuinely at the top of the scrolled viewport,
 * for the sticky-ancestor header to key off.
 *
 * Live-reported: the sticky header for a folder overlapped/garbled the real
 * row scrolled underneath it. Root cause: the caller used to re-derive this
 * index independently via `Math.floor(scrollOffset / rowHeight)`, assuming
 * every row's real rendered position exactly equals `index * rowHeight`.
 * `items` (from the virtualizer's own `getVirtualItems()`) is the single
 * source of truth for where rows actually are — reading it directly instead
 * can never drift from what is actually on screen. `items` includes
 * overscanned rows above and below the visible window, so the genuinely
 * topmost VISIBLE one is the first whose bottom edge is past `scrollOffset`.
 */
export function findTopVisibleItemIndex(
  items: readonly VirtualRowExtent[],
  scrollOffset: number,
): number {
  const topItem =
    items.find((item) => item.start + item.size > scrollOffset) ?? items[items.length - 1]
  return topItem ? topItem.index : -1
}

export function getStickyAncestorRow(
  rows: readonly VisibleFileTreeRow[],
  firstVisibleIndex: number,
): VisibleFileTreeRow | null {
  const ancestors = getStickyAncestorRows(rows, firstVisibleIndex)
  return ancestors[ancestors.length - 1] ?? null
}

export function getStickyAncestorRows(
  rows: readonly VisibleFileTreeRow[],
  firstVisibleIndex: number,
): VisibleFileTreeRow[] {
  const firstVisibleRow = rows[firstVisibleIndex]
  if (!firstVisibleRow || firstVisibleRow.depth === 0) {
    return []
  }

  const ancestors: Array<VisibleFileTreeRow | null> = Array.from(
    { length: firstVisibleRow.depth },
    () => null,
  )
  let remaining = firstVisibleRow.depth

  for (let index = firstVisibleIndex - 1; index >= 0 && remaining > 0; index--) {
    const candidate = rows[index]
    if (candidate.depth < firstVisibleRow.depth && ancestors[candidate.depth] === null) {
      ancestors[candidate.depth] = candidate
      remaining--
    }
  }

  return ancestors.filter((row): row is VisibleFileTreeRow => row !== null)
}

export interface StickyScrollLayout {
  /** Height (px) to reserve above the first rendered row. */
  paddingTop: number
  /** `items`, minus any rows that would render behind the sticky stack. */
  visibleItems: readonly VirtualRowExtent[]
}

/**
 * Where real content should start once a sticky-ancestor stack is showing,
 * so nothing renders partially behind its opaque band.
 *
 * Live-reported (twice): a folder name rendering with its first letters
 * sheared off directly under the sticky header. The stack is a
 * `position: sticky; height: 0` overlay (it has to be — the ancestor rows it
 * shows are virtualized OUT of `items`, so it can never be a normal-flow
 * sibling of them) painted at a FIXED screen position, while the row
 * scrolled to the real top of the content keeps rendering at its ordinary,
 * continuously-scrolled position underneath it. For whatever slice of every
 * scroll frame that position falls inside the stack's band, the stack
 * paints over the top of that row.
 *
 * A one-time size reservation (e.g. adding the stack's height to
 * `paddingTop` once) does NOT fix this — it only shifts which continuous
 * scroll position produces the overlap, since scrolling still passes
 * through every fractional offset between rows; verified live by scrubbing
 * scroll position by hand and watching the overlap reappear a few pixels
 * later. The only way nothing ever renders partially behind the stack is to
 * stop rendering the marker row (whatever row is currently at the real top)
 * at its raw scrolled offset, and instead SNAP it — and everything after —
 * to start exactly where the stack ends, the same "content snaps into
 * place under sticky headers" behavior editors with this feature already
 * use. `containerInset` must be the same value the stack's own CSS `top`
 * offset subtracts (`file-explorer-tree.css`'s `.file-tree-sticky-ancestors`
 * — matching it is what closes the last few pixels of the gap).
 */
export function computeStickyScrollLayout(
  items: readonly VirtualRowExtent[],
  stickyMarkerIndex: number,
  stickyAncestorCount: number,
  rowHeight: number,
  containerInset: number,
): StickyScrollLayout {
  const stickyStackHeight = stickyAncestorCount * rowHeight
  const markerItem =
    stickyAncestorCount > 0 ? items.find((item) => item.index === stickyMarkerIndex) : undefined

  if (!markerItem) {
    return { paddingTop: items.length ? items[0].start : 0, visibleItems: items }
  }

  return {
    paddingTop: markerItem.start + stickyStackHeight + containerInset,
    visibleItems: items.filter((item) => item.index >= stickyMarkerIndex),
  }
}

export function getGuideAncestorRows(
  rows: readonly VisibleFileTreeRow[],
  rowIndex: number,
): Array<VisibleFileTreeRow | null> {
  const row = rows[rowIndex]
  if (!row || row.depth === 0) {
    return []
  }

  const ancestors: Array<VisibleFileTreeRow | null> = Array.from({ length: row.depth }, () => null)
  let remaining = row.depth

  for (let index = rowIndex - 1; index >= 0 && remaining > 0; index--) {
    const candidate = rows[index]
    if (candidate.depth < row.depth && ancestors[candidate.depth] === null) {
      ancestors[candidate.depth] = candidate
      remaining--
    }
  }

  return ancestors
}
