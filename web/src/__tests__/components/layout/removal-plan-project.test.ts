/**
 * What removing a PROJECT means, worked out before anything is hidden.
 *
 * A project was the one row the planner refused outright — `draftFor` returned
 * null for it — so there was no gesture in the sidebar that could delete one.
 * The rule it now follows is the repo's, one level up: it takes everything under
 * it, and it never runs a clock.
 */
import { describe, it, expect } from 'vitest'
import { planRemoval } from '@/components/layout/removal-plan'
import type { Repo } from '@/lib/store/sidebar'

const PROJECTS = [
  { id: 'p1', name: 'harbour' },
  { id: 'p2', name: 'atlas' },
]

const repos: Repo[] = [
  {
    id: 'r1',
    projectId: 'p1',
    name: 'web',
    avatarLabel: 'W',
    avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'a', branch: 'alpha', status: 'new', age: '', order: 0 },
      { id: 'b', branch: 'beta', status: 'new', age: '', order: 1 },
    ],
    folders: [],
  },
  {
    id: 'r2',
    projectId: 'p1',
    name: 'api',
    avatarLabel: 'A',
    avatarColor: 'bg-teal-700',
    workspaces: [{ id: 'c', branch: 'gamma', status: 'new', age: '', order: 0 }],
    folders: [],
  },
  // Another project's repo, which must not be swept up.
  {
    id: 'r9',
    projectId: 'p2',
    name: 'infra',
    avatarLabel: 'I',
    avatarColor: 'bg-amber-700',
    workspaces: [{ id: 'z', branch: 'zeta', status: 'new', age: '', order: 0 }],
    folders: [],
  },
]

const planProject = (id: string) => planRemoval([{ kind: 'project', id }], repos, PROJECTS)

describe('planning a project removal', () => {
  it('hides the project row AND every repo row inside it', () => {
    // Hiding only the header would leave its repos on screen with nothing above
    // them — and the delete cascades server-side, so that would promise less
    // than is about to happen.
    const [draft] = planProject('p1')

    expect(draft.kind).toBe('project')
    expect(draft.id).toBe('p1')
    expect([...draft.hiddenIds].sort()).toEqual(['p1', 'r1', 'r2'])
  })

  it('leaves another project’s repos alone', () => {
    expect(planProject('p1')[0].hiddenIds).not.toContain('r9')
  })

  it('counts the repos AND every workspace under them', () => {
    // 2 repos + 3 workspaces.
    expect(planProject('p1')[0].extra).toBe(5)
  })

  it('carries no owning repo, because it spans all of them', () => {
    expect(planProject('p1')[0].repoId).toBe('')
    expect(planProject('p1')[0].projectId).toBe('p1')
  })

  it('reads as the project’s own name', () => {
    expect(planProject('p1')[0].label).toBe('harbour')
  })

  it('plans nothing for a project the list has never heard of', () => {
    expect(planRemoval([{ kind: 'project', id: 'nope' }], repos, PROJECTS)).toEqual([])
  })

  it('plans nothing when no project list is supplied at all', () => {
    // The old signature. Callers that never pass one keep the behaviour they
    // had rather than silently planning a removal with no label.
    expect(planRemoval([{ kind: 'project', id: 'p1' }], repos)).toEqual([])
  })
})
