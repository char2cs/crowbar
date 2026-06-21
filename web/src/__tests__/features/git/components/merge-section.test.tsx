import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

// ── Mock workspace context before importing the component ─────────────────────
const mockMergeStrategy = { value: 'merge' as import('@/features/workspace/stores/slices/branch-review-slice').MergeStrategy }
const mockStore = {
  getState: () => ({
    setBranchReviewMergeStrategy: vi.fn(),
  }),
}

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStoreContext: (selector: (s: { branchReview: { mergeStrategy: string } }) => unknown) =>
    selector({ branchReview: { mergeStrategy: mockMergeStrategy.value } }),
  useWorkspaceStore: () => mockStore,
}))

vi.mock('@/features/git/api/review-api', () => ({
  setMergeStrategy: vi.fn(() => Promise.resolve()),
  mergeIntoParent: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: {
    info: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    success: vi.fn(),
  },
}))

import { MergeSection } from '@/features/git/components/merge-section'

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderMergeSection(overrides: Partial<{
  canMergeLocally: boolean
  hasUncommitted: boolean
  status: string
  parentBranch: string
}> = {}) {
  const props = {
    wsId: 'ws-test',
    parentBranch: overrides.parentBranch ?? 'main',
    canMergeLocally: overrides.canMergeLocally ?? true,
    hasUncommitted: overrides.hasUncommitted ?? false,
    status: overrides.status ?? '',
  }
  return render(<MergeSection {...props} />)
}

describe('MergeSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('eligible state', () => {
    it('renders an enabled "Merge into main" button', () => {
      renderMergeSection({ canMergeLocally: true, hasUncommitted: false, status: '' })
      const btn = screen.getByRole('button', { name: /merge into main/i })
      expect(btn).toBeInTheDocument()
      expect(btn).not.toBeDisabled()
    })

    it('shows the eligibility line mentioning the parent branch', () => {
      renderMergeSection({
        canMergeLocally: true,
        hasUncommitted: false,
        status: '',
        parentBranch: 'develop',
      })
      expect(screen.getByText(/local/i)).toBeInTheDocument()
      expect(screen.getByText(/unprotected/i)).toBeInTheDocument()
    })
  })

  describe('uncommitted state', () => {
    it('renders a disabled button with commit-first copy', () => {
      renderMergeSection({ canMergeLocally: true, hasUncommitted: true, status: '' })
      const btn = screen.getByRole('button', { name: /commit your change/i })
      expect(btn).toBeInTheDocument()
      expect(btn).toBeDisabled()
    })

    it('does not render the merge action button', () => {
      renderMergeSection({ canMergeLocally: true, hasUncommitted: true, status: '' })
      expect(screen.queryByRole('button', { name: /merge into/i })).not.toBeInTheDocument()
    })
  })

  describe('protected state', () => {
    it('renders a disabled button mentioning pull request', () => {
      renderMergeSection({ canMergeLocally: false, hasUncommitted: false, status: '' })
      const btn = screen.getByRole('button', { name: /open a pull request/i })
      expect(btn).toBeInTheDocument()
      expect(btn).toBeDisabled()
    })

    it('includes parent branch name in the copy', () => {
      renderMergeSection({
        canMergeLocally: false,
        hasUncommitted: false,
        status: '',
        parentBranch: 'main',
      })
      const btn = screen.getByRole('button', { name: /main.*pull request/i })
      expect(btn).toBeDisabled()
    })
  })

  describe('conflict state', () => {
    it('renders a "Resolve conflicts" button', () => {
      renderMergeSection({
        canMergeLocally: true,
        hasUncommitted: false,
        status: 'pr-conflicts',
      })
      const btn = screen.getByRole('button', { name: /resolve conflicts/i })
      expect(btn).toBeInTheDocument()
      expect(btn).not.toBeDisabled()
    })

    it('does not render the merge action button', () => {
      renderMergeSection({
        canMergeLocally: true,
        hasUncommitted: false,
        status: 'pr-conflicts',
      })
      expect(screen.queryByRole('button', { name: /merge into/i })).not.toBeInTheDocument()
    })
  })

  describe('strategy selector', () => {
    it('renders the three strategy buttons', () => {
      renderMergeSection()
      expect(screen.getByRole('button', { name: /^merge$/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /^squash$/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /^rebase$/i })).toBeInTheDocument()
    })
  })
})
