import { describe, expect, it } from 'vitest'
import { placeholderKind, placeholderReason } from '@/lib/workspace/placeholder'
import type { Workspace } from '@/lib/store/sidebar'

// The daemon records a workspace with no checkout as a placeholder.
const ws = (over: Partial<Workspace> = {}): Workspace => ({
  id: 'w1',
  branch: 'develop',
  age: '',
  provisioning: over.localPath ? 'provisioned' : 'placeholder',
  ...over,
})

describe('placeholderKind', () => {
  it('is unprovisioned for a locked workspace with no localPath', () => {
    expect(placeholderKind(ws({ status: 'locked', heldByPath: '/repo' }), 'main')).toBe(
      'unprovisioned',
    )
  })
  it('is none for a locked workspace that has a localPath (healthy managed)', () => {
    expect(placeholderKind(ws({ status: 'locked', localPath: '/managed' }), 'main')).toBe('none')
  })
  it('is none for a healthy non-locked workspace', () => {
    expect(placeholderKind(ws({ status: 'new', localPath: '/managed' }), 'main')).toBe('none')
  })
  // An imported feature branch whose worktree could not be created is a
  // placeholder too, and it is deliberately NOT locked (locked survives
  // provisioning and would block merge/rename/delete forever). Requiring locked
  // here is what left the import's failed row unrecognised — no reason, no
  // Retry/Detach…, and no toast.
  it('is unprovisioned for an unlocked import placeholder held by another worktree', () => {
    expect(placeholderKind(ws({ status: 'new', heldByPath: '/Users/me/other' }), 'main')).toBe(
      'unprovisioned',
    )
  })

  // THE regression: `main` sitting in the repo's own main folder is the resting
  // state of every imported repo (adoptRepoHome adopts repo.Path in place;
  // provisioning then resolves holder.HeldByHome for that same branch). Read as
  // a failed provision it put a permanent amber "Branch needs provisioning"
  // triangle — and an error toast — on every repo's default branch, forever,
  // pointing at a Retry that RetryProvision refuses outright.
  it('is own-checkout when the branch is the repo’s own default branch', () => {
    const kind = placeholderKind(
      ws({ branch: 'main', status: 'locked', heldByPath: '/Users/me/repo-beta' }),
      'main',
    )
    expect(kind).toBe('own-checkout')
  })

  // The repo's own checkout is matched by BRANCH, never by comparing
  // `heldByPath` to the repo's path: git worktree list emits fully
  // symlink-resolved paths while the repo's own path is whatever folder the
  // user handed the importer, so the same directory routinely has two
  // spellings (/var vs /private/var on macOS).
  it('is own-checkout even when the holder path spells the repo root differently', () => {
    const kind = placeholderKind(
      ws({ branch: 'main', status: 'locked', heldByPath: '/private/var/x/repo-beta' }),
      'main',
    )
    expect(kind).toBe('own-checkout')
  })

  it('is unprovisioned for a non-default branch held by somebody else', () => {
    const kind = placeholderKind(
      ws({ branch: 'release/1.x', status: 'locked', heldByPath: '/Users/me/elsewhere' }),
      'main',
    )
    expect(kind).toBe('unprovisioned')
  })

  // A caller with no repo at all (project home) can own no checkout, so it can
  // never suppress a warning by accident.
  it('never reads as own-checkout without a repo default branch', () => {
    expect(placeholderKind(ws({ branch: 'main', heldByPath: '/repo' }), undefined)).toBe(
      'unprovisioned',
    )
    expect(placeholderKind(ws({ branch: '', heldByPath: '/repo' }), '')).toBe('unprovisioned')
  })
})

describe('placeholderReason', () => {
  it('names the branch and the holder path when known', () => {
    const w = ws({ status: 'locked', heldByPath: '/Users/me/repo' })
    const reason = placeholderReason(w, placeholderKind(w, 'main'))
    expect(reason).toContain('develop')
    expect(reason).toContain('/Users/me/repo')
  })
  it('falls back to a generic reason without a holder path', () => {
    const w = ws({ status: 'locked' })
    expect(placeholderReason(w, placeholderKind(w, 'main'))).toContain('develop')
  })
  // A failure with no live holder has no reconstructable reason, so the daemon
  // persists the cause on the row; showing "Retry to provision it" instead would
  // hide the only explanation the user can act on.
  it('prefers the recorded cause when there is no holder path', () => {
    const w = ws({ status: 'new', lastError: 'worktree add: disk full' })
    expect(placeholderReason(w, placeholderKind(w, 'main'))).toContain('worktree add: disk full')
  })
  it('still prefers the holder path over a recorded cause', () => {
    const w = ws({
      status: 'new',
      heldByPath: '/Users/me/other',
      lastError: 'branch_already_exists',
    })
    expect(placeholderReason(w, placeholderKind(w, 'main'))).toContain('/Users/me/other')
  })

  // The own-checkout line must not claim a failure, and must not name Retry:
  // RetryProvision refuses this case with ErrBranchStillHeld, so the only verb
  // that works is Detach.
  it('never says "couldn’t set up" or promises a Retry for the repo’s own checkout', () => {
    const w = ws({ branch: 'main', status: 'locked', heldByPath: '/Users/me/repo-beta' })
    const reason = placeholderReason(w, placeholderKind(w, 'main'))
    expect(reason).toContain('/Users/me/repo-beta')
    expect(reason).toContain('detach')
    expect(reason.toLowerCase()).not.toContain("couldn't")
    expect(reason.toLowerCase()).not.toContain('retry')
  })

  it('has nothing to say about a workspace with a worktree', () => {
    const w = ws({ localPath: '/managed' })
    expect(placeholderReason(w, placeholderKind(w, 'main'))).toBe('')
  })
})
