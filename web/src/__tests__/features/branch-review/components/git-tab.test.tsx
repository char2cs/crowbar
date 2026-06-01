import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen } from '@testing-library/react'
import { GitTab } from '@/features/branch-review/components/git-tab'

// ResizeObserver is not available in jsdom
beforeAll(() => {
  global.ResizeObserver = vi.fn().mockImplementation((_cb) => ({
    observe: vi.fn(),
    unobserve: vi.fn(),
    disconnect: vi.fn(),
  }))
})

vi.mock('@/features/git/stores/git-store', () => ({
  useGitStore: vi.fn((selector) => selector({
    gitStatus: { files: [], ahead: 0, behind: 0 },
    stashes: [],
    actions: { setStashes: vi.fn(), refreshWorkspaceGitStatus: vi.fn() },
  })),
}))

vi.mock('@/features/git/stores/git-repository-store', () => ({
  useRepositoryStore: Object.assign(
    vi.fn((selector) => selector({ activeRepoPath: '/mock/repo' })),
    { use: { activeRepoPath: () => '/mock/repo' } }
  ),
}))

vi.mock('@/features/settings/store', () => ({
  useSettingsStore: vi.fn((selector) => selector({
    settings: { showUntrackedFiles: true },
  })),
}))

vi.mock('@/features/git/hooks/use-git-file-diff-stats', () => ({
  useGitFileDiffStats: vi.fn(() => ({})),
}))

vi.mock('@/features/branch-review/components/commits-tab', () => ({
  CommitsTab: vi.fn(() => <div data-testid="commits-tab" />),
}))

vi.mock('@/features/git/components/git-commit-panel', () => ({
  default: vi.fn(({ stagedFilesCount: _stagedFilesCount }: { stagedFilesCount: number }) => (
    <textarea placeholder="Commit message..." />
  )),
}))

vi.mock('@/features/git/components/status/git-status-panel', () => ({
  default: vi.fn(() => <div data-testid="git-status-panel" />),
}))

vi.mock('@/features/git/components/git-stash-command-surface', () => ({
  GitStashCommandSurface: vi.fn(() => null),
}))

vi.mock('@/features/git/components/git-remote-manager', () => ({
  default: vi.fn(() => null),
}))

vi.mock('@/features/git/components/git-tag-manager', () => ({
  default: vi.fn(() => null),
}))

describe('GitTab', () => {
  it('renders without crashing', () => {
    render(<GitTab wsId="ws-1" />)
    // The commit panel textarea should be present
    expect(screen.getByPlaceholderText('Commit message...')).toBeDefined()
  })
})
