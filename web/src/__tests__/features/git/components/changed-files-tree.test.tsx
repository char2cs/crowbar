import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ChangedFilesTree } from '@/features/git/components/changed-files-tree'
import { SIDEBAR_FILE_ROW_HEIGHT } from '@/components/sidebar/lib/sidebar-file-row-height'
import { viewedMapOf } from '@/__tests__/__fixtures__/viewed'
import type { GitDiff } from '@/features/git/types/git-types'

// ── Mock dependencies ────────────────────────────────────────────────────────

const mocks = vi.hoisted(() => {
  const settingsState = {
    settings: {
      compactGitStatusBadges: false,
      iconTheme: 'default',
    },
  }
  return { settingsState }
})

vi.mock('@/features/settings/store', () => {
  const useSettingsStore = Object.assign(
    (selector: (s: typeof mocks.settingsState) => unknown) => selector(mocks.settingsState),
    { getState: () => mocks.settingsState },
  )
  return { useSettingsStore }
})

// FileExplorerIcon uses useSettingsStore + icon-theme-registry; stub it out so
// tests focus on tree structure rather than icon rendering internals.
vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: ({ fileName, isDir }: { fileName: string; isDir?: boolean }) => (
    <span data-testid="file-icon" data-dir={isDir ? 'true' : 'false'}>
      {fileName}
    </span>
  ),
}))

// ── Layout ───────────────────────────────────────────────────────────────────

// The tree is virtualised, and a virtualiser with a zero-height scroll element
// has no window to render into — jsdom has no layout engine, so every element
// measures 0×0 and nothing at all would mount. Give elements a sidebar-sized
// rect before render; the fixtures here are small enough to fit in it whole, so
// every assertion below is about tree structure, exactly as before.
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

// ── Fixtures ─────────────────────────────────────────────────────────────────

function makeDiff(file_path: string, overrides: Partial<GitDiff> = {}): GitDiff {
  return {
    file_path,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: [],
    additions: 2,
    deletions: 1,
    ...overrides,
  }
}

/** 3-file diff: 2 committed, 1 uncommitted */
const THREE_FILE_DIFFS: GitDiff[] = [
  makeDiff('src/api/client.ts', { additions: 5, deletions: 2 }),
  makeDiff('src/utils/helpers.ts', { additions: 1, deletions: 0 }),
  makeDiff('src/api/server.ts', { uncommitted: true, additions: 3, deletions: 1 }),
]

const NAV = { viewed: {}, onSetViewed: vi.fn(), openThreadCounts: new Map<string, number>() }

// ── ChangedFilesTree tests ────────────────────────────────────────────────────

describe('ChangedFilesTree', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the directory structure (src/api and src/utils folders)', () => {
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={vi.fn()} {...NAV} />)

    // Folders are rendered as SidebarFileRow buttons; folder names appear as text
    // (and also in the mocked FileExplorerIcon span), so use getAllByText.
    expect(screen.getAllByText('api').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('utils').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('src').length).toBeGreaterThanOrEqual(1)
  })

  it('renders leaf file names inside the tree', () => {
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={vi.fn()} {...NAV} />)

    // File names are derived from path (last segment shown by the row)
    expect(screen.getByTitle('src/api/client.ts')).toBeInTheDocument()
    expect(screen.getByTitle('src/utils/helpers.ts')).toBeInTheDocument()
    expect(screen.getByTitle('src/api/server.ts')).toBeInTheDocument()
  })

  it('renders exactly one amber "uncommitted" pill', () => {
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={vi.fn()} {...NAV} />)

    const pills = screen.getAllByText('uncommitted')
    expect(pills).toHaveLength(1)
  })

  it('calls onFileOpen with the correct file_path when a leaf is clicked', async () => {
    const user = userEvent.setup()
    const onFileOpen = vi.fn()
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={onFileOpen} {...NAV} />)

    // The file row carries title={file.path}
    const clientRow = screen.getByTitle('src/api/client.ts')
    await user.click(clientRow)

    expect(onFileOpen).toHaveBeenCalledTimes(1)
    expect(onFileOpen).toHaveBeenCalledWith('src/api/client.ts')
  })

  it('calls onFileOpen with the uncommitted file_path when its leaf is clicked', async () => {
    const user = userEvent.setup()
    const onFileOpen = vi.fn()
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={onFileOpen} {...NAV} />)

    const serverRow = screen.getByTitle('src/api/server.ts')
    await user.click(serverRow)

    expect(onFileOpen).toHaveBeenCalledWith('src/api/server.ts')
  })

  it('renders no uncommitted pill when no files have uncommitted:true', () => {
    const committedDiffs = THREE_FILE_DIFFS.map((d) => ({ ...d, uncommitted: false }))
    render(<ChangedFilesTree files={committedDiffs} onFileOpen={vi.fn()} {...NAV} />)

    expect(screen.queryByText('uncommitted')).not.toBeInTheDocument()
  })

  it('collapses a folder when its row is clicked', async () => {
    const user = userEvent.setup()
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={vi.fn()} {...NAV} />)

    // Folder 'src' row is a button — there may be multiple elements with that
    // text (mock icon + label span); find the one that is itself a button or
    // whose closest button is at depth 0.
    const srcButtons = screen
      .getAllByText('src')
      .map((el) => el.closest('button'))
      .filter(Boolean) as HTMLButtonElement[]
    const srcFolderRow = srcButtons[0]!

    expect(screen.getByTitle('src/api/client.ts')).toBeInTheDocument()

    await user.click(srcFolderRow)

    // After collapsing 'src', none of its children should be visible
    expect(screen.queryByTitle('src/api/client.ts')).not.toBeInTheDocument()
  })

  it('renders empty without crashing when files array is empty', () => {
    const { container } = render(<ChangedFilesTree files={[]} onFileOpen={vi.fn()} {...NAV} />)
    expect(container.firstChild).toBeInTheDocument()
  })

  it("marks each file with the explorer's git status letter", () => {
    render(
      <ChangedFilesTree
        files={[
          makeDiff('a/new.ts', { is_new: true }),
          makeDiff('a/gone.ts', { is_deleted: true }),
          makeDiff('a/edit.ts'),
          makeDiff('a/moved.ts', { is_renamed: true }),
        ]}
        onFileOpen={vi.fn()}
        {...NAV}
      />,
    )
    expect(screen.getByLabelText('Added')).toHaveTextContent('A')
    expect(screen.getByLabelText('Deleted')).toHaveTextContent('D')
    expect(screen.getByLabelText('Modified')).toHaveTextContent('M')
    expect(screen.getByLabelText('Renamed')).toHaveTextContent('R')
  })

  it('formats the trailing +N -N with the shared change-count formatter', () => {
    render(
      <ChangedFilesTree
        files={[makeDiff('big.ts', { additions: 1500, deletions: 12 })]}
        onFileOpen={vi.fn()}
        {...NAV}
      />,
    )
    const row = screen.getByTitle('big.ts')
    expect(row).toHaveTextContent('+2k')
    expect(row).toHaveTextContent('-12')
  })

  it('renders folder and file rows on the shared 26px pitch the virtualiser lays out', () => {
    const many = Array.from({ length: 200 }, (_, i) =>
      makeDiff(`f${String(i).padStart(3, '0')}.ts`),
    )
    const { container } = render(<ChangedFilesTree files={many} onFileOpen={vi.fn()} {...NAV} />)
    const rows = screen.getAllByRole('button')
    expect(rows.length).toBeLessThan(many.length)
    for (const row of rows) expect(row.style.height).toBe(`${SIDEBAR_FILE_ROW_HEIGHT}px`)

    // scroll container > list > [top spacer, ...rows, bottom spacer]
    const list = container.firstElementChild?.firstElementChild as HTMLElement
    const spacers = [list.firstElementChild, list.lastElementChild] as HTMLElement[]
    const spacerHeight = spacers.reduce((sum, el) => sum + Number.parseFloat(el.style.height), 0)
    expect(spacerHeight + rows.length * SIDEBAR_FILE_ROW_HEIGHT).toBe(
      many.length * SIDEBAR_FILE_ROW_HEIGHT,
    )
  })

  it('reports each folder row as expanded or collapsed', async () => {
    const user = userEvent.setup()
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={vi.fn()} {...NAV} />)
    const api = screen.getByRole('button', { name: /^api/ })
    expect(api).toHaveAttribute('aria-expanded', 'true')
    await user.click(api)
    expect(screen.getByRole('button', { name: /^api/ })).toHaveAttribute('aria-expanded', 'false')
  })
})

describe('ChangedFilesTree review navigator', () => {
  const [client, , server] = THREE_FILE_DIFFS

  it('folds single-child folder chains into one row keyed by the full chain', async () => {
    const user = userEvent.setup()
    const deep = [makeDiff('a/b/c/x.ts'), makeDiff('a/b/c/y.ts')]
    render(<ChangedFilesTree files={deep} onFileOpen={vi.fn()} {...NAV} />)

    const chain = screen.getByText('a/b/c').closest('button')!
    expect(screen.queryByText('a')).not.toBeInTheDocument()
    expect(screen.getByTitle('a/b/c/x.ts')).toBeInTheDocument()

    await user.click(chain)
    expect(screen.queryByTitle('a/b/c/x.ts')).not.toBeInTheDocument()
  })

  it('checks a viewed file and toggles it without opening the diff', async () => {
    const user = userEvent.setup()
    const onFileOpen = vi.fn()
    const onSetViewed = vi.fn()
    render(
      <ChangedFilesTree
        files={THREE_FILE_DIFFS}
        onFileOpen={onFileOpen}
        viewed={await viewedMapOf([client])}
        onSetViewed={onSetViewed}
        openThreadCounts={new Map()}
      />,
    )

    expect(screen.getByRole('checkbox', { name: 'Mark client.ts as viewed' })).toBeChecked()
    const server_ = screen.getByRole('checkbox', { name: 'Mark server.ts as viewed' })
    expect(server_).not.toBeChecked()

    await user.click(server_)
    expect(onSetViewed).toHaveBeenCalledWith([server], true)
    expect(onFileOpen).not.toHaveBeenCalled()
  })

  it('keeps the viewed checkbox out of the row button and toggles it from the keyboard', async () => {
    const user = userEvent.setup()
    const onFileOpen = vi.fn()
    const onSetViewed = vi.fn()
    render(
      <ChangedFilesTree
        files={THREE_FILE_DIFFS}
        onFileOpen={onFileOpen}
        viewed={{}}
        onSetViewed={onSetViewed}
        openThreadCounts={new Map()}
      />,
    )

    for (const box of screen.getAllByRole('checkbox')) {
      expect(box.closest('button')).toBeNull()
    }
    const server_ = screen.getByRole('checkbox', { name: 'Mark server.ts as viewed' })
    server_.focus()
    await user.keyboard(' ')
    expect(onSetViewed).toHaveBeenCalledWith([server], true)
    expect(onFileOpen).not.toHaveBeenCalled()
  })

  it('shows folder progress as a tri-state and toggles every descendant in one batch', async () => {
    const user = userEvent.setup()
    const onSetViewed = vi.fn()
    const { rerender } = render(
      <ChangedFilesTree
        files={THREE_FILE_DIFFS}
        onFileOpen={vi.fn()}
        viewed={await viewedMapOf([client])}
        onSetViewed={onSetViewed}
        openThreadCounts={new Map()}
      />,
    )

    const api = screen.getByRole('checkbox', { name: 'Mark api as viewed' })
    expect(api).toHaveAttribute('aria-checked', 'mixed')
    expect(screen.getByText('1/2')).toBeInTheDocument()
    expect(screen.getByText('1/3')).toBeInTheDocument()

    await user.click(api)
    expect(onSetViewed).toHaveBeenCalledTimes(1)
    expect(onSetViewed.mock.calls[0][0]).toEqual(expect.arrayContaining([client, server]))
    expect(onSetViewed.mock.calls[0][0]).toHaveLength(2)
    expect(onSetViewed.mock.calls[0][1]).toBe(true)

    rerender(
      <ChangedFilesTree
        files={THREE_FILE_DIFFS}
        onFileOpen={vi.fn()}
        viewed={await viewedMapOf([client, server])}
        onSetViewed={onSetViewed}
        openThreadCounts={new Map()}
      />,
    )
    expect(screen.getByRole('checkbox', { name: 'Mark api as viewed' })).toBeChecked()
    await user.click(screen.getByRole('checkbox', { name: 'Mark api as viewed' }))
    expect(onSetViewed.mock.calls[1][1]).toBe(false)
  })

  it('badges only files that have open threads', () => {
    render(
      <ChangedFilesTree
        files={THREE_FILE_DIFFS}
        onFileOpen={vi.fn()}
        {...{ viewed: {}, onSetViewed: vi.fn() }}
        openThreadCounts={new Map([[client.file_path, 2]])}
      />,
    )
    expect(screen.getByLabelText('2 open threads')).toBeInTheDocument()
    expect(screen.getAllByLabelText(/open thread/)).toHaveLength(1)
  })
})
