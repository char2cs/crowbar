import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { GitFileItem } from '@/features/git/components/status/git-status-file-item'
import type { GitFile } from '@/features/git/types/git-types'

// FileExplorerIcon subscribes to the settings store for the icon theme; the row
// itself does not. Only the store is stubbed — the real icon component renders,
// because the point of this file is that its anchor really reaches the DOM.
const mocks = vi.hoisted(() => ({
  settingsState: { settings: { compactGitStatusBadges: false, iconTheme: 'default' } },
}))

vi.mock('@/features/settings/store', () => ({
  useSettingsStore: Object.assign(
    (selector: (s: typeof mocks.settingsState) => unknown) => selector(mocks.settingsState),
    { getState: () => mocks.settingsState },
  ),
}))

const FILE: GitFile = {
  path: 'src/features/terminal/resolve-terminal-connection.ts',
  status: 'modified',
  staged: false,
}

/**
 * Every anchor the Phase 1 gate names (`native/oracle/ANCHORS.md`) must be
 * reachable by `data-oracle-id` alone — the extractor never looks at anything
 * else, so an untagged element is invisible to the oracle.
 */
describe('git status row oracle anchors', () => {
  it('tags all nine anchors on a row rendered in the fullest state', () => {
    const { container } = render(
      <GitFileItem
        file={FILE}
        diffStats={{ additions: 12, deletions: 3 }}
        showDirectory
        showFileIcon
        indentLevel={2}
        uncommitted
        compactGitStatusBadges={false}
      />,
    )

    const ids = Array.from(container.querySelectorAll('[data-oracle-id]')).map((el) =>
      el.getAttribute('data-oracle-id'),
    )

    expect(new Set(ids)).toEqual(
      new Set([
        'git-row-item',
        'git-row-button',
        'git-row-icon',
        'git-row-name',
        'git-row-dir',
        'git-row-badge',
        'git-row-added',
        'git-row-deleted',
        'git-row-guide-0',
        'git-row-guide-1',
      ]),
    )
  })

  it('puts the root anchor on the container that owns the ::before background', () => {
    const { container } = render(<GitFileItem file={FILE} compactGitStatusBadges={false} />)
    const root = container.querySelector('[data-oracle-id="git-row-item"]')

    expect(root?.classList.contains('file-tree-item')).toBe(true)
    // The button is a descendant of the root, so the extractor's walk reaches it.
    expect(root?.querySelector('[data-oracle-id="git-row-button"]')?.tagName).toBe('BUTTON')
  })

  it('emits one guide anchor per indent level, numbered from zero', () => {
    const { container } = render(
      <GitFileItem file={FILE} indentLevel={3} compactGitStatusBadges={false} />,
    )
    const guides = Array.from(container.querySelectorAll('[data-oracle-id^="git-row-guide-"]')).map(
      (el) => el.getAttribute('data-oracle-id'),
    )

    expect(guides).toEqual(['git-row-guide-0', 'git-row-guide-1', 'git-row-guide-2'])
  })

  it('carries the text anchors verbatim, before any visual truncation', () => {
    const { container } = render(
      <GitFileItem
        file={FILE}
        diffStats={{ additions: 12, deletions: 3 }}
        showDirectory
        compactGitStatusBadges={false}
      />,
    )

    expect(container.querySelector('[data-oracle-id="git-row-name"]')?.textContent).toBe(
      'resolve-terminal-connection.ts',
    )
    expect(container.querySelector('[data-oracle-id="git-row-dir"]')?.textContent).toBe(
      'src/features/terminal',
    )
    expect(container.querySelector('[data-oracle-id="git-row-added"]')?.textContent).toBe('+12')
    expect(container.querySelector('[data-oracle-id="git-row-deleted"]')?.textContent).toBe('-3')
  })

  it('adds nothing to the class list — the anchors are inert markers', () => {
    const { container } = render(
      <GitFileItem file={FILE} showFileIcon compactGitStatusBadges={false} />,
    )

    for (const el of Array.from(container.querySelectorAll('[data-oracle-id]'))) {
      expect(el.className.toString()).not.toContain('oracle')
    }
  })
})
