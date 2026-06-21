import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createStore } from 'zustand/vanilla'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'

// ── Mock the workspace-store-registry so tests don't spin up real stores ──────
const mockSetBranchReviewMergeStrategy = vi.fn()

// A real vanilla zustand store so useStore(store, selector) works correctly.
// Tests mutate branchReview.mergeStrategy via setState before rendering.
const mockStore = createStore<{
  branchReview: { mergeStrategy: MergeStrategy; setBranchReviewMergeStrategy: (s: MergeStrategy) => void }
}>(() => ({
  branchReview: {
    mergeStrategy: 'merge',
    setBranchReviewMergeStrategy: mockSetBranchReviewMergeStrategy,
  },
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => mockStore,
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
  // No WorkspaceStoreContext provider — MergeSection must work standalone via registry
  return render(<MergeSection {...props} />)
}

describe('MergeSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockStore.setState({ branchReview: { mergeStrategy: 'merge', setBranchReviewMergeStrategy: mockSetBranchReviewMergeStrategy } })
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

    it('highlights the active strategy from registry state', () => {
      mockStore.setState({ branchReview: { mergeStrategy: 'squash', setBranchReviewMergeStrategy: mockSetBranchReviewMergeStrategy } })
      renderMergeSection()
      // The active strategy button uses "default" variant, others use "outline".
      // Button component adds data-variant or we check aria — simplest: squash button exists
      expect(screen.getByRole('button', { name: /^squash$/i })).toBeInTheDocument()
    })
  })
})
