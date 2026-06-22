import { describe, it, expect } from 'vitest'
import type {
  WorkspaceDTO,
  RepoDTO,
  ProjectDTO,
  ThreadDTO,
  ThreadReplyDTO,
  TerminalSessionDTO,
} from '@/lib/types'

// These DTOs are the camelCase mirrors of the Go Canonical Contracts. The asserts
// are type-level (the file fails to compile if a field is missing/renamed) plus a
// trivial runtime check so vitest counts the suite.
describe('canonical DTO shapes', () => {
  it('WorkspaceDTO carries the §5 7-value status union and fields', () => {
    const ws: WorkspaceDTO = {
      id: 'w1',
      repoId: 'r1',
      projectId: 'p1',
      branch: 'feature/x',
      parentId: '',
      forkPointSha: '',
      status: 'pr-conflicts',
      working: true,
      lastError: '',
      added: 1,
      deleted: 2,
      mergeStrategy: 'rebase',
      canMergeLocally: true,
      mergeConflicts: false,
      parentBranch: 'main',
      prUrl: '',
      prTitle: '',
      prTargetBranch: '',
    }
    const statuses: WorkspaceDTO['status'][] = [
      'new',
      'locked',
      'pr-conflicts',
      'deleted',
      'pr-merged',
      'pr-open',
      'pr-closed',
    ]
    expect(ws.id).toBe('w1')
    expect(statuses).toHaveLength(7)
  })

  it('RepoDTO carries avatarEmoji', () => {
    const repo: RepoDTO = {
      id: 'r1',
      projectId: 'p1',
      name: 'crowbar',
      path: '/tmp/crowbar',
      defaultBranch: 'main',
      avatarLabel: 'C',
      avatarColor: 'bg-indigo-700',
      avatarUrl: '',
      avatarEmoji: 'rocket',
    }
    expect(repo.avatarEmoji).toBe('rocket')
  })

  it('ProjectDTO, ThreadDTO, ThreadReplyDTO, TerminalSessionDTO compile', () => {
    const reply: ThreadReplyDTO = {
      id: 'rp1',
      threadId: 't1',
      body: 'hi',
      author: 'me',
      isAgent: false,
      createdAt: '2026-06-19T00:00:00Z',
    }
    const thread: ThreadDTO = {
      id: 't1',
      projectId: 'p1',
      repoId: 'r1',
      workspaceId: 'w1',
      filePath: 'a.ts',
      line: 1,
      startLine: 1,
      endLine: 1,
      side: 'new',
      messageId: 'm0',
      body: 'comment',
      author: 'me',
      isAgent: false,
      resolved: false,
      createdAt: '2026-06-19T00:00:00Z',
      replies: [reply],
    }
    const project: ProjectDTO = {
      id: 'p1',
      name: 'crowbar',
      path: '/tmp/crowbar',
      status: '',
      lastActivity: '2026-06-19T00:00:00Z',
    }
    const term: TerminalSessionDTO = {
      id: 'ts1',
      projectId: 'p1',
      repoId: 'r1',
      workspaceId: 'w1',
      profileId: '',
      status: 'active',
      createdAt: '2026-06-19T00:00:00Z',
      endedAt: null,
    }
    expect(thread.replies[0]).toBe(reply)
    expect(project.id).toBe('p1')
    expect(term.status).toBe('active')
  })
})
