import { render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ChangedFilesTree } from '@/features/git/components/changed-files-tree'
import type { GitDiff } from '@/features/git/types/git-types'

/**
 * Scale gate for the sidebar changed-files tree, backing
 * docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md.
 *
 * Cause 1 in that spec: `sidebar-carousel.tsx` mounts all four panels
 * permanently — confirmed live, all four report `display: flex` /
 * `visibility: visible`, none dormant — so `GitPanel` renders on every tab.
 * Before Phase 1 `ChangedFilesTree` was unvirtualised, so its DOM cost was
 * O(changed files) whether or not the user could see it: 500 files produced
 * 500 rows and 6,292 DOM nodes.
 *
 * These tests asserted that defect as a tripwire until Phase 1 virtualised the
 * tree; they now assert the TARGET behaviour — the mounted row count is bounded
 * by the virtual window, not by the input. A regression to eager rendering
 * fails here.
 *
 * DOM node count is used rather than wall time deliberately — it is
 * deterministic, so this is a real gate instead of a flaky timing assertion.
 */

const mocks = vi.hoisted(() => ({
  settingsState: { settings: { compactGitStatusBadges: false, iconTheme: 'default' } },
}))

vi.mock('@/features/settings/store', () => {
  const useSettingsStore = Object.assign(
    (selector: (s: typeof mocks.settingsState) => unknown) => selector(mocks.settingsState),
    { getState: () => mocks.settingsState },
  )
  return { useSettingsStore }
})

vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: ({ fileName }: { fileName: string }) => <span>{fileName}</span>,
}))

// jsdom has no layout engine, so every element measures 0×0 and the virtualiser
// would collapse to its overscan alone. Give elements a sidebar-sized rect
// before render so the window under test is the realistic one the app produces.
// Purely geometric — no timers, no polling.
const VIEWPORT_WIDTH = 280
const VIEWPORT_HEIGHT = 800
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect

beforeEach(() => {
  const rect = {
    top: 0,
    left: 0,
    right: VIEWPORT_WIDTH,
    bottom: VIEWPORT_HEIGHT,
    width: VIEWPORT_WIDTH,
    height: VIEWPORT_HEIGHT,
    x: 0,
    y: 0,
  }
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect
})

function makeFiles(count: number): GitDiff[] {
  return Array.from({ length: count }, (_, i) => ({
    file_path: `src/pkg${i % 20}/file${i}.ts`,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: [],
    additions: 10,
    deletions: 4,
    uncommitted: i % 3 === 0,
  }))
}

function countRows(container: HTMLElement) {
  return {
    // One GitFileItem renders one row; the title attribute carries the path.
    fileRows: container.querySelectorAll('[title^="src/"]').length,
    domNodes: container.querySelectorAll('*').length,
  }
}

describe('ChangedFilesTree scale characterisation', () => {
  it('renders a bounded number of rows regardless of file count', () => {
    const fileCount = 500
    const { container } = render(
      <ChangedFilesTree files={makeFiles(fileCount)} repoPath="/repo" onFileOpen={() => {}} />,
    )

    const { fileRows, domNodes } = countRows(container)

    // A scale test should report its measurement, not just gate on it — this
    // number is what Phase 1 is judged against (baseline: 500 rows / 6,292
    // nodes / 12.6 nodes per file).
    console.info(
      `[scale] ${fileCount} files -> ${fileRows} rows, ${domNodes} DOM nodes ` +
        `(${(domNodes / fileCount).toFixed(1)} nodes/file)`,
    )

    // Bounded by the virtual window, not the input.
    expect(fileRows).toBeLessThan(100)
    expect(fileRows).toBeGreaterThan(0)
  })

  it('row count does not scale with file count', () => {
    const small = render(
      <ChangedFilesTree files={makeFiles(100)} repoPath="/repo" onFileOpen={() => {}} />,
    )
    const large = render(
      <ChangedFilesTree files={makeFiles(400)} repoPath="/repo" onFileOpen={() => {}} />,
    )

    const smallCount = countRows(small.container).fileRows
    const largeCount = countRows(large.container).fileRows

    expect(smallCount).toBeGreaterThan(0)
    expect(largeCount).toBeGreaterThan(0)

    // 4x the files, the same window. Was exactly 4.0 before virtualisation.
    expect(largeCount / smallCount).toBeLessThan(1.5)
  })
})
