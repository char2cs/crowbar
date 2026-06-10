// web/src/__tests__/lib/mock/git-data.test.ts
import { describe, it, expect } from 'vitest'
import { getMockGitStatus, getMockCommitHistory, getMockBranches } from '@/lib/mock/git-data'

describe('getMockGitStatus', () => {
  it('returns a files array with staged/unstaged entries', () => {
    const status = getMockGitStatus('/repos/crowbar')
    expect(Array.isArray(status.files)).toBe(true)
    expect(status.files.length).toBeGreaterThan(0)
    expect(typeof status.branch).toBe('string')
  })
  it('each file has path, status, and staged flag', () => {
    const status = getMockGitStatus('/repos/crowbar')
    status.files.forEach((f) => {
      expect(typeof f.path).toBe('string')
      expect(typeof f.status).toBe('string')
      expect(typeof f.staged).toBe('boolean')
    })
  })
  it('returns different data per repo', () => {
    const crowbar = getMockGitStatus('/repos/crowbar')
    const core = getMockGitStatus('/repos/quiver-core')
    expect(crowbar.branch).not.toBe(core.branch)
  })
})

describe('getMockCommitHistory', () => {
  it('returns at least 3 commits', () => {
    const commits = getMockCommitHistory('/repo')
    expect(commits.length).toBeGreaterThanOrEqual(3)
  })
  it('each commit has required fields', () => {
    getMockCommitHistory('/repo').forEach((c) => {
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
    const current = getMockBranches('/repo').filter((b) => b.isCurrent)
    expect(current.length).toBe(1)
  })
})
