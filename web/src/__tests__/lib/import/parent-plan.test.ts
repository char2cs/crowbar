import { describe, it, expect } from 'vitest'
import { computeImportPlan } from '@/lib/import/parent-plan'

const branches = [
  { name: 'dev', isProtected: true, hasWorkspace: false },
  { name: 'feat/base', isProtected: false, hasWorkspace: false },
  { name: 'feat/9324', isProtected: false, hasWorkspace: false },
]
const prLinks = [
  { head: 'feat/9324', base: 'feat/base' },
  { head: 'feat/base', base: 'dev' },
]

describe('computeImportPlan', () => {
  it('counts the missing PR ancestor as a created parent', () => {
    const plan = computeImportPlan(['feat/9324'], prLinks, branches, 'dev')
    expect(plan).toEqual({ importCount: 1, parentCount: 1 }) // feat/base
  })

  it('does not double-count an ancestor that is itself selected', () => {
    const plan = computeImportPlan(['feat/9324', 'feat/base'], prLinks, branches, 'dev')
    expect(plan).toEqual({ importCount: 2, parentCount: 0 })
  })

  it('terminates at an already-imported ancestor', () => {
    const imported = branches.map((b) =>
      b.name === 'feat/base' ? { ...b, hasWorkspace: true } : b,
    )
    const plan = computeImportPlan(['feat/9324'], prLinks, imported, 'dev')
    expect(plan).toEqual({ importCount: 1, parentCount: 0 })
  })

  it('terminates at the default branch without counting it', () => {
    const plan = computeImportPlan(['feat/base'], prLinks, branches, 'dev')
    expect(plan).toEqual({ importCount: 1, parentCount: 0 })
  })

  it('treats a branch with no PR as a lone import', () => {
    const plan = computeImportPlan(['feat/9324'], [], branches, 'dev')
    expect(plan).toEqual({ importCount: 1, parentCount: 0 })
  })

  it('breaks PR-base cycles instead of looping forever', () => {
    const cyc = [
      { head: 'a', base: 'b' },
      { head: 'b', base: 'a' },
    ]
    const cycBranches = [
      { name: 'a', isProtected: false, hasWorkspace: false },
      { name: 'b', isProtected: false, hasWorkspace: false },
    ]
    const plan = computeImportPlan(['a'], cyc, cycBranches, 'main')
    // a→b (created), b→a but a is visited → stop. Only b is a created parent.
    expect(plan).toEqual({ importCount: 1, parentCount: 1 })
  })
})
