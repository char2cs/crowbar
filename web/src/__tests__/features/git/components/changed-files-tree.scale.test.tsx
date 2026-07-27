import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ChangedFilesTree } from '@/features/git/components/changed-files-tree'
import type { GitDiff } from '@/features/git/types/git-types'

/**
 * Scale characterisation for the sidebar changed-files tree, backing the
 * attribution phase of
 * docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md.
 *
 * Cause 1 in that spec: `sidebar-carousel.tsx` mounts all four panels
 * permanently — confirmed live, all four report `display: flex` /
 * `visibility: visible`, none dormant — so `GitPanel` renders on every tab.
 * `ChangedFilesTree` is unvirtualised, so its DOM cost is O(changed files)
 * whether or not the user can see it.
 *
 * These tests assert the CURRENT behaviour on purpose. They are a tripwire,
 * not an endorsement: Phase 1 virtualises this tree, at which point the
 * `rendersOneRowPerFile` expectation MUST fail and be inverted to a bounded
 * row count. A green suite after that change would mean the fix did not land.
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
  it('rendersOneRowPerFile — O(changed files), the defect Phase 1 removes', () => {
    const fileCount = 500
    const { container } = render(
      <ChangedFilesTree files={makeFiles(fileCount)} repoPath="/repo" onFileOpen={() => {}} />,
    )

    const { fileRows, domNodes } = countRows(container)

    // A characterisation test should report its measurement, not just gate on
    // it — this number is what Phase 1 is judged against.
    console.info(
      `[scale] ${fileCount} files -> ${fileRows} rows, ${domNodes} DOM nodes ` +
        `(${(domNodes / fileCount).toFixed(1)} nodes/file)`,
    )

    // Every file is in the DOM, regardless of viewport. This is the defect.
    expect(fileRows).toBe(fileCount)
    // Each row costs many nodes, so the real cost is a multiple of file count.
    expect(domNodes).toBeGreaterThan(fileCount * 4)
  })

  it('scales linearly with file count — cost is not bounded by a viewport', () => {
    const small = render(
      <ChangedFilesTree files={makeFiles(100)} repoPath="/repo" onFileOpen={() => {}} />,
    )
    const large = render(
      <ChangedFilesTree files={makeFiles(400)} repoPath="/repo" onFileOpen={() => {}} />,
    )

    const smallCount = countRows(small.container).fileRows
    const largeCount = countRows(large.container).fileRows

    // 4x the files, 4x the rows. A virtualised tree would hold this ratio near
    // 1 because both renders would be clamped to the same visible window.
    expect(largeCount / smallCount).toBe(4)
  })
})
