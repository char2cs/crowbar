import { describe, it, expect } from 'vitest'
import { findWorkspaceForBranch } from '@/lib/workspace/branch-workspace'
import type { Repo, Workspace } from '@/lib/store/sidebar'

function w(id: string, branch: string): Workspace {
  return { id, branch, status: 'new', added: 0, deleted: 0, working: false,
    canMergeLocally: false, mergeConflicts: false, lastError: '', age: '' } as Workspace
}
const repo: Repo = {
  id: 'r1', projectId: 'p1', name: 'crowbar',
  avatarLabel: 'C', avatarColor: 'bg-sky-700',
  defaultWorkspaceId: 'd', defaultBranch: 'develop',
  workspaces: [w('c1', 'feature/x'), w('c2', 'spike/y')],
} as Repo

describe('findWorkspaceForBranch', () => {
  it('matches an existing managed child branch', () => {
    expect(findWorkspaceForBranch(repo, 'feature/x')).toBe('c1')
  })
  it('does NOT match the default branch — the repo folder is unmanaged', () => {
    expect(findWorkspaceForBranch(repo, 'develop')).toBeNull()
  })
  it('returns null for a free branch', () => {
    expect(findWorkspaceForBranch(repo, 'feature/new')).toBeNull()
  })
  it('is case-sensitive', () => {
    expect(findWorkspaceForBranch(repo, 'Feature/X')).toBeNull()
  })
  it('trims surrounding whitespace before matching', () => {
    expect(findWorkspaceForBranch(repo, '  feature/x  ')).toBe('c1')
  })
  it('returns null for an empty/whitespace branch', () => {
    expect(findWorkspaceForBranch(repo, '   ')).toBeNull()
  })
})
