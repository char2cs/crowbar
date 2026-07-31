import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FileExplorerTreeItem } from '@/features/file-explorer/file-explorer/components/file-explorer-tree-item'
import type { FileTreeGitStatusDecoration } from '@/features/file-explorer/lib/file-tree-git-status'
import type { FileEntry } from '@/features/file-system/types/app'

// The row reads the clipboard store for cut state and the icon reads the
// settings store for the icon theme. Only the stores are stubbed — the real
// icon component renders, because the point of this file is that its anchor
// really reaches the DOM.
vi.mock('@/features/file-explorer/stores/file-explorer-clipboard-store', () => ({
  useFileClipboardStore: () => false,
}))

const mocks = vi.hoisted(() => ({
  settingsState: { settings: { iconTheme: 'default' } },
}))

vi.mock('@/features/settings/store', () => ({
  useSettingsStore: Object.assign(
    (selector: (s: typeof mocks.settingsState) => unknown) => selector(mocks.settingsState),
    { getState: () => mocks.settingsState },
  ),
}))

const FILE: FileEntry = {
  name: 'resolve-terminal-connection.ts',
  path: '/repo/src/features/terminal/lib/resolve-terminal-connection.ts',
  isDir: false,
}

const noDecoration = (_file: FileEntry): FileTreeGitStatusDecoration | null => null

const props = {
  file: FILE,
  depth: 2,
  guideTargets: [],
  previousDepth: 2,
  nextDepth: 2,
  indentSize: 16,
  density: 'default' as const,
  isExpanded: false,
  isActive: false,
  dragOverPath: null,
  isDragging: false,
  editingValue: '',
  onEditingValueChange: vi.fn(),
  onKeyDown: vi.fn(),
  onBlur: vi.fn(),
  getGitStatusDecoration: noDecoration,
}

/**
 * The `file-tree-row` surface — `native/oracle/ANCHORS.md`.
 *
 * The second gate surface, and the one the §8.3 **state** axis is measured on.
 * Its anchors must be reachable by `data-oracle-id` alone: the extractor never
 * looks at anything else, so an untagged element is invisible to the oracle.
 */
describe('file tree row oracle anchors', () => {
  it('tags every anchor the surface names', () => {
    const { container } = render(<FileExplorerTreeItem {...props} />)

    const ids = Array.from(container.querySelectorAll('[data-oracle-id]')).map((el) =>
      el.getAttribute('data-oracle-id'),
    )

    expect(new Set(ids)).toEqual(
      new Set([
        'file-row-item',
        'file-row-button',
        'file-row-icon',
        'file-row-name',
        'file-row-guide-0',
        'file-row-guide-1',
      ]),
    )
  })

  it('puts the root anchor on the container that owns the ::before background', () => {
    const { container } = render(<FileExplorerTreeItem {...props} />)
    const root = container.querySelector('[data-oracle-id="file-row-item"]')

    expect(root?.classList.contains('file-tree-item')).toBe(true)
    // The button is a descendant of the root, so the extractor's walk reaches it.
    expect(root?.querySelector('[data-oracle-id="file-row-button"]')?.tagName).toBe('BUTTON')
  })

  it('emits one guide anchor per indent level, numbered from zero', () => {
    const { container } = render(<FileExplorerTreeItem {...props} depth={3} />)
    const guides = Array.from(
      container.querySelectorAll('[data-oracle-id^="file-row-guide-"]'),
    ).map((el) => el.getAttribute('data-oracle-id'))

    expect(guides).toEqual(['file-row-guide-0', 'file-row-guide-1', 'file-row-guide-2'])
  })

  it('carries the name verbatim, before any visual truncation', () => {
    const { container } = render(<FileExplorerTreeItem {...props} />)

    expect(container.querySelector('[data-oracle-id="file-row-name"]')?.textContent).toBe(
      'resolve-terminal-connection.ts',
    )
  })

  /**
   * `native/oracle/ANCHORS.md` v1.5 and v1.6, on the name and on nothing else.
   *
   * The same two declarations are on the GPUI side in `crowbar-ui`'s
   * `file_tree_row::{CONTENT_SIZED, LINE_SIZED}`. A declaration on one side only
   * is a `FieldPresence` delta that forgives nothing, so this list drifting is a
   * gate failure a long way from here — which is why it is pinned at both ends.
   *
   * The **button** is the trap this test holds shut. It paints text (it is where
   * the font is declared) and it holds exactly one line, which reads like the
   * same case as the name — but `h-6` authors its border box at 24px around that
   * line's 20px. Declaring it would compare 24 against 20 and manufacture a 4px
   * delta on an anchor both engines agree on, exactly as declaring the git row's
   * badge would have.
   */
  it('declares content_sized and line_sized on the name, and on nothing else', () => {
    const { container } = render(<FileExplorerTreeItem {...props} />)

    const declared = (attribute: string) =>
      Array.from(container.querySelectorAll(`[${attribute}]`)).map((el) =>
        el.getAttribute('data-oracle-id'),
      )

    expect(declared('data-oracle-content-sized')).toEqual(['file-row-name'])
    expect(declared('data-oracle-line-sized')).toEqual(['file-row-name'])

    for (const id of ['file-row-item', 'file-row-button', 'file-row-icon', 'file-row-guide-0']) {
      const el = container.querySelector(`[data-oracle-id="${id}"]`)
      expect(el?.hasAttribute('data-oracle-content-sized'), id).toBe(false)
      expect(el?.hasAttribute('data-oracle-line-sized'), id).toBe(false)
    }
  })

  /**
   * **The reason this surface exists.** `data-active` is what
   * `.file-tree-item[data-active='true']::before` keys off, and it is set from
   * the `isActive` prop on every render — where `SidebarTreeRow`'s equivalent is
   * dead, because neither live consumer passes its `active` prop.
   */
  it('sets data-active on the root anchor from isActive, which is what ::before reads', () => {
    const resting = render(<FileExplorerTreeItem {...props} isActive={false} />)
    const selected = render(<FileExplorerTreeItem {...props} isActive />)

    const root = (result: ReturnType<typeof render>) =>
      result.container.querySelector('[data-oracle-id="file-row-item"]')

    expect(root(resting)?.getAttribute('data-active')).toBeNull()
    expect(root(selected)?.getAttribute('data-active')).toBe('true')
    // And it is the *root* that carries it, not the button: the pseudo-element
    // that paints belongs to `.file-tree-item`.
    expect(
      selected.container
        .querySelector('[data-oracle-id="file-row-button"]')
        ?.getAttribute('data-active'),
    ).toBeNull()
    expect(
      selected.container
        .querySelector('[data-oracle-id="file-row-button"]')
        ?.getAttribute('aria-selected'),
    ).toBe('true')
  })

  /**
   * The inline-editing branch renders a different row — an `<Input>` where the
   * button is — and is deliberately **not** tagged. The extractor takes the
   * first match for its root, so a second `file-row-item` in the document would
   * be indistinguishable from the real one and would snapshot a shape the native
   * surface does not model.
   */
  it('leaves the inline-editing row untagged', () => {
    const { container } = render(
      <FileExplorerTreeItem {...props} file={{ ...FILE, isEditing: true }} />,
    )

    expect(container.querySelectorAll('[data-oracle-id]')).toHaveLength(0)
  })

  it('adds nothing to the class list — the anchors are inert markers', () => {
    const { container } = render(<FileExplorerTreeItem {...props} isActive />)

    for (const el of Array.from(container.querySelectorAll('[data-oracle-id]'))) {
      expect(el.className.toString()).not.toContain('oracle')
    }
  })
})
