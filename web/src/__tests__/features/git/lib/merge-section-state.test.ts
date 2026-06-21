import { describe, it, expect } from 'vitest'
import { resolveMergeState } from '@/features/git/lib/merge-section-state'

describe('resolveMergeState', () => {
  describe('eligible', () => {
    it('returns eligible when canMergeLocally=true, no uncommitted, no conflict', () => {
      const result = resolveMergeState({
        canMergeLocally: true,
        hasUncommitted: false,
        status: 'pr-open',
      })
      expect(result.kind).toBe('eligible')
      expect(result.reason).toMatch(/unprotected/i)
    })

    it('returns eligible for empty status string when otherwise clear', () => {
      const result = resolveMergeState({
        canMergeLocally: true,
        hasUncommitted: false,
        status: '',
      })
      expect(result.kind).toBe('eligible')
    })
  })

  describe('uncommitted', () => {
    it('returns uncommitted when canMergeLocally=true but hasUncommitted=true', () => {
      const result = resolveMergeState({
        canMergeLocally: true,
        hasUncommitted: true,
        status: 'new',
      })
      expect(result.kind).toBe('uncommitted')
      expect(result.reason).toBeTruthy()
    })
  })

  describe('protected', () => {
    it('returns protected when canMergeLocally=false and no conflict', () => {
      const result = resolveMergeState({
        canMergeLocally: false,
        hasUncommitted: false,
        status: 'pr-open',
      })
      expect(result.kind).toBe('protected')
      expect(result.reason).toBeTruthy()
    })

    it('returns protected over uncommitted when canMergeLocally=false and hasUncommitted=true', () => {
      const result = resolveMergeState({
        canMergeLocally: false,
        hasUncommitted: true,
        status: 'new',
      })
      expect(result.kind).toBe('protected')
    })
  })

  describe('conflict', () => {
    it('returns conflict when status is pr-conflicts', () => {
      const result = resolveMergeState({
        canMergeLocally: true,
        hasUncommitted: false,
        status: 'pr-conflicts',
      })
      expect(result.kind).toBe('conflict')
      expect(result.reason).toBeTruthy()
    })

    it('conflict takes precedence over protected (canMergeLocally=false)', () => {
      const result = resolveMergeState({
        canMergeLocally: false,
        hasUncommitted: false,
        status: 'pr-conflicts',
      })
      expect(result.kind).toBe('conflict')
    })

    it('conflict takes precedence over uncommitted', () => {
      const result = resolveMergeState({
        canMergeLocally: false,
        hasUncommitted: true,
        status: 'pr-conflicts',
      })
      expect(result.kind).toBe('conflict')
    })
  })
})
