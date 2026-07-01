import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ChangedFilesTree } from '@/features/git/components/changed-files-tree'
import { GitFileItem } from '@/features/git/components/status/git-status-file-item'
import type { GitDiff, GitFile } from '@/features/git/types/git-types'

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

// ── ChangedFilesTree tests ────────────────────────────────────────────────────

describe('ChangedFilesTree', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the directory structure (src/api and src/utils folders)', () => {
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={vi.fn()} />)

    // Folders are rendered as SidebarTreeRow buttons; folder names appear as text
    // (and also in the mocked FileExplorerIcon span), so use getAllByText.
    expect(screen.getAllByText('api').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('utils').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('src').length).toBeGreaterThanOrEqual(1)
  })

  it('renders leaf file names inside the tree', () => {
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={vi.fn()} />)

    // File names are derived from path (last segment shown by GitFileItem)
    expect(screen.getByTitle('src/api/client.ts')).toBeInTheDocument()
    expect(screen.getByTitle('src/utils/helpers.ts')).toBeInTheDocument()
    expect(screen.getByTitle('src/api/server.ts')).toBeInTheDocument()
  })

  it('renders exactly one amber "uncommitted" pill', () => {
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={vi.fn()} />)

    const pills = screen.getAllByText('uncommitted')
    expect(pills).toHaveLength(1)
  })

  it('calls onFileOpen with the correct file_path when a leaf is clicked', async () => {
    const user = userEvent.setup()
    const onFileOpen = vi.fn()
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={onFileOpen} />)

    // GitFileItem renders the file row with title={file.path}
    const clientRow = screen.getByTitle('src/api/client.ts')
    await user.click(clientRow)

    expect(onFileOpen).toHaveBeenCalledTimes(1)
    expect(onFileOpen).toHaveBeenCalledWith('src/api/client.ts')
  })

  it('calls onFileOpen with the uncommitted file_path when its leaf is clicked', async () => {
    const user = userEvent.setup()
    const onFileOpen = vi.fn()
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={onFileOpen} />)

    const serverRow = screen.getByTitle('src/api/server.ts')
    await user.click(serverRow)

    expect(onFileOpen).toHaveBeenCalledWith('src/api/server.ts')
  })

  it('renders no uncommitted pill when no files have uncommitted:true', () => {
    const committedDiffs = THREE_FILE_DIFFS.map((d) => ({ ...d, uncommitted: false }))
    render(<ChangedFilesTree files={committedDiffs} onFileOpen={vi.fn()} />)

    expect(screen.queryByText('uncommitted')).not.toBeInTheDocument()
  })

  it('collapses a folder when its row is clicked', async () => {
    const user = userEvent.setup()
    render(<ChangedFilesTree files={THREE_FILE_DIFFS} onFileOpen={vi.fn()} />)

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
    const { container } = render(<ChangedFilesTree files={[]} onFileOpen={vi.fn()} />)
    expect(container.firstChild).toBeInTheDocument()
  })
})

// ── GitFileItem uncommitted pill tests ────────────────────────────────────────

const BASE_FILE: GitFile = {
  path: 'src/foo.ts',
  status: 'modified',
  staged: false,
}

describe('GitFileItem uncommitted pill', () => {
  it('renders an "uncommitted" badge when uncommitted is true', () => {
    render(<GitFileItem file={BASE_FILE} uncommitted />)
    expect(screen.getByText('uncommitted')).toBeInTheDocument()
  })

  it('does NOT render the badge when uncommitted is false', () => {
    render(<GitFileItem file={BASE_FILE} uncommitted={false} />)
    expect(screen.queryByText('uncommitted')).not.toBeInTheDocument()
  })

  it('does NOT render the badge when uncommitted prop is omitted', () => {
    render(<GitFileItem file={BASE_FILE} />)
    expect(screen.queryByText('uncommitted')).not.toBeInTheDocument()
  })
})
