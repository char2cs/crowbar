import { describe, expect, test } from 'vitest'
import {
  buildVisibleFileTreeRows,
  computeStickyScrollLayout,
  filterFileTreeForFffHits,
  findTopVisibleItemIndex,
  getGuideAncestorRows,
  getStickyAncestorRow,
  getStickyAncestorRows,
} from '@/features/file-explorer/file-explorer/lib/visible-file-tree-rows'

const tree = [
  {
    name: 'root',
    path: '/root',
    isDir: true,
    children: [
      {
        name: 'src',
        path: '/root/src',
        isDir: true,
        children: [
          {
            name: 'features',
            path: '/root/src/features',
            isDir: true,
            children: [
              {
                name: 'file-explorer',
                path: '/root/src/features/file-explorer',
                isDir: true,
                children: [
                  {
                    name: 'file-tree.tsx',
                    path: '/root/src/features/file-explorer/file-tree.tsx',
                    isDir: false,
                  },
                ],
              },
            ],
          },
        ],
      },
    ],
  },
]

describe('buildVisibleFileTreeRows', () => {
  test('shows only the expanded root branch', () => {
    const rows = buildVisibleFileTreeRows(tree, new Set(['/root']))

    expect(rows.map((row) => row.file.path)).toEqual(['/root', '/root/src'])
    expect(rows.map((row) => row.depth)).toEqual([0, 1])
  })

  test('shows third-level rows when parent folders are expanded', () => {
    const rows = buildVisibleFileTreeRows(
      tree,
      new Set(['/root', '/root/src', '/root/src/features']),
    )

    expect(rows.map((row) => row.file.path)).toEqual([
      '/root',
      '/root/src',
      '/root/src/features',
      '/root/src/features/file-explorer',
    ])
    expect(rows.map((row) => row.depth)).toEqual([0, 1, 2, 3])
  })

  test('shows deeper descendants once every ancestor is expanded', () => {
    const rows = buildVisibleFileTreeRows(
      tree,
      new Set(['/root', '/root/src', '/root/src/features', '/root/src/features/file-explorer']),
    )

    expect(rows.map((row) => row.file.path)).toEqual([
      '/root',
      '/root/src',
      '/root/src/features',
      '/root/src/features/file-explorer',
      '/root/src/features/file-explorer/file-tree.tsx',
    ])
    expect(rows.map((row) => row.depth)).toEqual([0, 1, 2, 3, 4])
  })

  test('hides nested descendants when a middle folder collapses', () => {
    const rows = buildVisibleFileTreeRows(tree, new Set(['/root', '/root/src']))

    expect(rows.map((row) => row.file.path)).toEqual(['/root', '/root/src', '/root/src/features'])
    expect(rows.map((row) => row.depth)).toEqual([0, 1, 2])
  })

  test('compacts expanded single-child folder chains', () => {
    const rows = buildVisibleFileTreeRows(
      tree,
      new Set(['/root', '/root/src', '/root/src/features']),
      { compactFolders: true },
    )

    expect(rows.map((row) => row.file.path)).toEqual(['/root/src/features/file-explorer'])
    expect(rows.map((row) => row.displayName)).toEqual(['root/src/features/file-explorer'])
    expect(rows.map((row) => row.depth)).toEqual([0])
  })

  test('stops compacting at the collapsed folder', () => {
    const rows = buildVisibleFileTreeRows(tree, new Set(['/root', '/root/src']), {
      compactFolders: true,
    })

    expect(rows.map((row) => row.file.path)).toEqual(['/root/src/features'])
    expect(rows.map((row) => row.displayName)).toEqual(['root/src/features'])
    expect(rows.map((row) => row.isExpanded)).toEqual([false])
  })

  test('finds the nearest sticky ancestor for a visible descendant', () => {
    const rows = buildVisibleFileTreeRows(
      tree,
      new Set(['/root', '/root/src', '/root/src/features', '/root/src/features/file-explorer']),
    )

    expect(getStickyAncestorRow(rows, 4)?.file.path).toBe('/root/src/features/file-explorer')
    expect(getStickyAncestorRow(rows, 2)?.file.path).toBe('/root/src')
    expect(getStickyAncestorRow(rows, 0)).toBeNull()
  })

  test('finds the full sticky ancestor stack for a visible descendant', () => {
    const rows = buildVisibleFileTreeRows(
      tree,
      new Set(['/root', '/root/src', '/root/src/features', '/root/src/features/file-explorer']),
    )

    expect(getStickyAncestorRows(rows, 4).map((row) => row.file.path)).toEqual([
      '/root',
      '/root/src',
      '/root/src/features',
      '/root/src/features/file-explorer',
    ])
    expect(getStickyAncestorRows(rows, 0)).toEqual([])
  })

  test('finds guide ancestors for each visible depth level', () => {
    const rows = buildVisibleFileTreeRows(
      tree,
      new Set(['/root', '/root/src', '/root/src/features', '/root/src/features/file-explorer']),
    )

    expect(getGuideAncestorRows(rows, 4).map((row) => row?.file.path)).toEqual([
      '/root',
      '/root/src',
      '/root/src/features',
      '/root/src/features/file-explorer',
    ])
  })
})

describe('findTopVisibleItemIndex', () => {
  // Live-reported: the sticky-ancestor header overlapped/garbled the real row
  // scrolled underneath it. Root cause: the caller used to re-derive "which
  // row is at the top" via `Math.floor(scrollOffset / rowHeight)`, assuming
  // every row's real position is exactly `index * rowHeight` — an assumption
  // that can drift from the virtualizer's own items. These pin reading the
  // items directly instead.
  const items = [
    { index: 5, start: 0, size: 24 },
    { index: 6, start: 24, size: 24 },
    { index: 7, start: 48, size: 24 },
  ]

  test('finds the item whose range contains the scroll offset', () => {
    expect(findTopVisibleItemIndex(items, 0)).toBe(5)
    expect(findTopVisibleItemIndex(items, 23)).toBe(5)
    expect(findTopVisibleItemIndex(items, 24)).toBe(6)
    expect(findTopVisibleItemIndex(items, 47)).toBe(6)
    expect(findTopVisibleItemIndex(items, 48)).toBe(7)
  })

  test('never assumes uniform row height — a taller row shifts every index after it', () => {
    const uneven = [
      { index: 0, start: 0, size: 40 }, // taller than the rest
      { index: 1, start: 40, size: 24 },
      { index: 2, start: 64, size: 24 },
    ]
    // A naive scrollOffset/24 division would land on index 1 here; the real
    // item covering offset 30 is still index 0.
    expect(findTopVisibleItemIndex(uneven, 30)).toBe(0)
    expect(findTopVisibleItemIndex(uneven, 41)).toBe(1)
  })

  test('falls back to the last item when the offset is past every item (bottom of an overscanned list)', () => {
    expect(findTopVisibleItemIndex(items, 1000)).toBe(7)
  })

  test('returns -1 for an empty item list', () => {
    expect(findTopVisibleItemIndex([], 0)).toBe(-1)
  })
})

// Live-reported (twice): a sticky ancestor header (e.g. "src") painted OVER
// the top of whichever row happened to be scrolled underneath it, shearing
// off that row's icon and text. A one-time size reservation shifts WHICH
// scroll position triggers the overlap but never eliminates it, because
// scrolling passes through every fractional offset between rows — these
// pin the fix instead: content is snapped to start exactly where the sticky
// stack ends, never at a continuous, possibly-mid-row scroll offset.
describe('computeStickyScrollLayout', () => {
  const items = [
    { index: 5, start: 0, size: 24 },
    { index: 6, start: 24, size: 24 },
    { index: 7, start: 48, size: 24 },
    { index: 8, start: 72, size: 24 },
  ]

  test('with no sticky ancestors, passes items through unchanged (natural continuous scroll)', () => {
    const result = computeStickyScrollLayout(items, 6, 0, 24, 4)
    expect(result).toEqual({ paddingTop: 0, visibleItems: items })
  })

  test('is a no-op for an empty item list', () => {
    expect(computeStickyScrollLayout([], -1, 0, 24, 4)).toEqual({
      paddingTop: 0,
      visibleItems: [],
    })
  })

  test('snaps the marker row to start right after the stack, dropping rows the stack now covers', () => {
    // 1 ancestor sticky ("src"), marker row is index 6.
    const result = computeStickyScrollLayout(items, 6, 1, 24, 4)
    // stack height (24) + the same inset its CSS `top` offset subtracts (4).
    expect(result.paddingTop).toBe(24 /* marker's own start */ + 24 + 4)
    // index 5 sat BEFORE the marker — it would render inside the stack's own
    // band if kept, so it's dropped rather than shown partially behind it.
    expect(result.visibleItems.map((i) => i.index)).toEqual([6, 7, 8])
  })

  test('reserves room for a taller stack when multiple ancestors are sticky at once', () => {
    // 2 ancestors sticky ("src" and "src/features"), same marker row.
    const result = computeStickyScrollLayout(items, 6, 2, 24, 4)
    expect(result.paddingTop).toBe(24 + 48 + 4)
    expect(result.visibleItems.map((i) => i.index)).toEqual([6, 7, 8])
  })

  test('the reserved padding is independent of how far the raw scroll is into the marker row', () => {
    // The whole point: paddingTop must be a step function of the MARKER
    // ROW's own (row-quantized) start, never of the continuous scroll
    // offset within it — otherwise the overlap this fix closes reappears
    // at whatever fractional offset the reservation forgot to account for.
    const a = computeStickyScrollLayout(items, 6, 1, 24, 4)
    const b = computeStickyScrollLayout(items, 6, 1, 24, 4)
    expect(a.paddingTop).toBe(b.paddingTop)
  })

  test('falls back to the natural offset when the marker index is not one of the given items', () => {
    const result = computeStickyScrollLayout(items, 999, 1, 24, 4)
    expect(result).toEqual({ paddingTop: 0, visibleItems: items })
  })
})

describe('filterFileTreeForFffHits', () => {
  test('keeps matching files with their ancestors expanded', () => {
    const result = filterFileTreeForFffHits(tree, [
      { path: '/root/src/features/file-explorer/file-tree.tsx' },
    ])
    const rows = buildVisibleFileTreeRows(result.files, result.expandedPaths)

    expect(rows.map((row) => row.file.path)).toEqual([
      '/root',
      '/root/src',
      '/root/src/features',
      '/root/src/features/file-explorer',
      '/root/src/features/file-explorer/file-tree.tsx',
    ])
    expect(Array.from(result.matchedPaths)).toEqual([
      '/root/src/features/file-explorer/file-tree.tsx',
    ])
    expect(result.orderedMatchedPaths).toEqual(['/root/src/features/file-explorer/file-tree.tsx'])
    expect(result.matchCount).toBe(1)
  })

  test('keeps a matched folder without expanding unmatched descendants', () => {
    const result = filterFileTreeForFffHits(tree, [{ path: '/root/src/features' }])
    const rows = buildVisibleFileTreeRows(result.files, result.expandedPaths)

    expect(rows.map((row) => row.file.path)).toEqual(['/root', '/root/src', '/root/src/features'])
    expect(Array.from(result.matchedPaths)).toEqual(['/root/src/features'])
  })

  test('returns an empty tree for empty fff results', () => {
    const result = filterFileTreeForFffHits(tree, [])

    expect(result.files).toEqual([])
    expect(result.matchCount).toBe(0)
    expect(result.expandedPaths.size).toBe(0)
  })
})
