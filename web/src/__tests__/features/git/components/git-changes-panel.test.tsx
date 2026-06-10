import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { GitChangesPanel } from '@/features/git/components/git-changes-panel'
import type { GitFile } from '@/features/git/types/git-types'

const mocks = vi.hoisted(() => {
  const openContent = vi.fn(() => 'buffer-1')
  const handleFileOpen = vi.fn(() => Promise.resolve())
  const getFileDiff = vi.fn()
  const gitState = {
    gitStatus: {
      branch: 'main',
      ahead: 0,
      behind: 0,
      files: [] as GitFile[],
    },
    commits: [],
    actions: { reload: vi.fn(() => Promise.resolve()) },
  }
  const settingsState = { settings: { openDiffOnClick: true } }
  const workspaceStore = {
    getState: () => ({ bufferActions: { openContent }, buffers: [] }),
    setState: vi.fn(),
  }
  return { openContent, handleFileOpen, getFileDiff, gitState, settingsState, workspaceStore }
})

vi.mock('@/features/git/components/git-commit-panel', () => ({
  default: () => <div data-testid="commit-panel" />,
}))

vi.mock('@/features/git/stores/git-store', () => {
  const useGitStore = Object.assign(
    (selector: (s: typeof mocks.gitState) => unknown) => selector(mocks.gitState),
    { getState: () => mocks.gitState },
  )
  return { useGitStore }
})

vi.mock('@/features/settings/store', () => {
  const useSettingsStore = Object.assign(
    (selector: (s: typeof mocks.settingsState) => unknown) => selector(mocks.settingsState),
    { getState: () => mocks.settingsState },
  )
  return { useSettingsStore }
})

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'ws-1',
}))

vi.mock('@/features/workspace/stores/workspace-store-ref', () => ({
  getActiveWorkspaceStoreRef: () => mocks.workspaceStore,
}))

vi.mock('@/features/file-system/controllers/store', () => ({
  useFileSystemStore: { getState: () => ({ handleFileOpen: mocks.handleFileOpen }) },
}))

vi.mock('@/features/git/api/git-diff-api', () => ({
  getFileDiff: mocks.getFileDiff,
  getCommitDiff: vi.fn(),
  getRefDiff: vi.fn(),
  getStashDiff: vi.fn(),
}))

vi.mock('@/features/git/api/git-status-api', () => ({
  discardFileChanges: vi.fn(),
  stageAllFiles: vi.fn(),
  stageFile: vi.fn(),
  unstageAllFiles: vi.fn(),
  unstageFile: vi.fn(),
}))

vi.mock('@/components/ui/primitive-dialog-service', () => ({
  primitiveAlert: vi.fn(() => Promise.resolve()),
}))

const fakeDiff = (filePath: string) => ({
  file_path: filePath,
  old_path: filePath,
  new_path: filePath,
  is_new: false,
  is_deleted: false,
  is_renamed: false,
  lines: [
    { line_type: 'removed', content: 'old line', old_line_number: 1 },
    { line_type: 'added', content: 'new line', new_line_number: 1 },
  ],
  additions: 1,
  deletions: 1,
})

describe('GitChangesPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.settingsState.settings.openDiffOnClick = true
    mocks.gitState.gitStatus.files = []
    mocks.getFileDiff.mockImplementation((_wsId: string, path: string) =>
      Promise.resolve(fakeDiff(path)),
    )
  })

  it('opens a diff buffer when an unstaged changed file is clicked', async () => {
    mocks.gitState.gitStatus.files = [{ path: 'README.md', status: 'modified', staged: false }]
    render(<GitChangesPanel />)

    await userEvent.click(screen.getByTitle('README.md — open diff'))

    await waitFor(() => expect(mocks.openContent).toHaveBeenCalledTimes(1))
    expect(mocks.getFileDiff).toHaveBeenCalledWith('ws-1', 'README.md', false)
    expect(mocks.openContent).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'diff',
        path: 'diff://working-tree/all-files',
        diffData: expect.objectContaining({
          fileKeys: ['unstaged:README.md'],
          initiallyExpandedFileKey: 'unstaged:README.md',
        }),
      }),
    )
  })

  it('requests the staged diff for files in the Staged section', async () => {
    mocks.gitState.gitStatus.files = [{ path: 'src/app.ts', status: 'modified', staged: true }]
    render(<GitChangesPanel />)

    await userEvent.click(screen.getByTitle('src/app.ts — open diff'))

    await waitFor(() => expect(mocks.openContent).toHaveBeenCalledTimes(1))
    expect(mocks.getFileDiff).toHaveBeenCalledWith('ws-1', 'src/app.ts', true)
    expect(mocks.openContent).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'diff',
        diffData: expect.objectContaining({
          initiallyExpandedFileKey: 'staged:src/app.ts',
        }),
      }),
    )
  })

  it('opens the file directly when openDiffOnClick is disabled', async () => {
    mocks.settingsState.settings.openDiffOnClick = false
    mocks.gitState.gitStatus.files = [{ path: 'README.md', status: 'modified', staged: false }]
    render(<GitChangesPanel />)

    await userEvent.click(screen.getByTitle('README.md — open diff'))

    await waitFor(() => expect(mocks.handleFileOpen).toHaveBeenCalledWith('README.md', false))
    expect(mocks.openContent).not.toHaveBeenCalled()
  })
})
