// web/src/__tests__/lib/mock/git-data.test.ts
import { describe, it, expect } from 'vitest'
import { getMockGitStatus, getMockCommitHistory, getMockBranches } from '@/lib/mock/git-data'

describe('getMockGitStatus', () => {
  it('returns staged and unstaged files', () => {
    const status = getMockGitStatus('/repo')
    expect(Array.isArray(status.staged)).toBe(true)
    expect(Array.isArray(status.unstaged)).toBe(true)
    expect(status.staged.length + status.unstaged.length).toBeGreaterThan(0)
  })
  it('each file has path and status', () => {
    const status = getMockGitStatus('/repo')
    ;[...status.staged, ...status.unstaged].forEach(f => {
      expect(typeof f.path).toBe('string')
      expect(typeof f.status).toBe('string')
    })
  })
})

describe('getMockCommitHistory', () => {
  it('returns at least 3 commits', () => {
    const commits = getMockCommitHistory('/repo')
    expect(commits.length).toBeGreaterThanOrEqual(3)
  })
  it('each commit has required fields', () => {
    getMockCommitHistory('/repo').forEach(c => {
      expect(typeof c.hash).toBe('string')
      expect(typeof c.message).toBe('string')
      expect(typeof c.author).toBe('string')
      expect(typeof c.date).toBe('string')
    })
  })
})

describe('getMockBranches', () => {
  it('returns at least one branch', () => {
    expect(getMockBranches('/repo').length).toBeGreaterThan(0)
  })
  it('exactly one branch is current', () => {
    const current = getMockBranches('/repo').filter(b => b.isCurrent)
    expect(current.length).toBe(1)
  })
})
