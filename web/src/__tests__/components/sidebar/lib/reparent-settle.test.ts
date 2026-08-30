import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import {
  watchReparent,
  ReparentFailedError,
  ReparentTimeoutError,
} from '@/components/sidebar/lib/reparent-settle'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'

function makeRepo(): Repo {
  return {
    id: 'repo-1',
    projectId: 'proj-1',
    name: 'repo-1',
    avatarLabel: 'R',
    avatarColor: 'bg-indigo-700',
    workspaces: [{ id: 'ws-fork', branch: 'fork', age: '', parentId: 'ws-a' }],
  }
}

function patchWorkspace(patch: Record<string, unknown>): void {
  useSidebarStore.setState((s) => ({
    repos: s.repos.map((r) => ({
      ...r,
      workspaces: r.workspaces.map((w) => (w.id === 'ws-fork' ? { ...w, ...patch } : w)),
    })),
  }))
}

beforeEach(() => {
  useSidebarStore.setState({ ...getInitialState(), repos: [makeRepo()] })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('watchReparent', () => {
  it('resolves immediately when a NEW object already reports the target parentId', async () => {
    const wait = watchReparent('ws-fork', 'ws-b')
    patchWorkspace({ parentId: 'ws-b' })

    await expect(wait()).resolves.toBeUndefined()
  })

  it('resolves once a later store update reports the new parentId (subscription path)', async () => {
    const wait = watchReparent('ws-fork', 'ws-b')
    const p = wait()

    let settled = false
    p.then(() => {
      settled = true
    })
    await Promise.resolve()
    await Promise.resolve()
    expect(settled).toBe(false)

    patchWorkspace({ parentId: 'ws-b' })

    await expect(p).resolves.toBeUndefined()
  })

  it('rejects with ReparentFailedError, naming the workspace and reason, when lastError CHANGES', async () => {
    const wait = watchReparent('ws-fork', 'ws-b')
    const p = wait()

    patchWorkspace({ lastError: 'workspace has fork children' })

    await expect(p).rejects.toThrow(ReparentFailedError)
    await expect(p).rejects.toThrow('reparent of ws-fork failed: workspace has fork children')
  })

  it('does not treat a PRE-EXISTING lastError as a fresh failure', async () => {
    patchWorkspace({ lastError: 'stale unrelated error' })
    const wait = watchReparent('ws-fork', 'ws-b')
    const p = wait()

    let settled = false
    let rejected = false
    p.then(
      () => {
        settled = true
      },
      () => {
        rejected = true
      },
    )

    // A frame that repeats the SAME error (a reconnect reseed, say) must not
    // resolve or reject on its own.
    patchWorkspace({ lastError: 'stale unrelated error', age: '1m' })
    await Promise.resolve()
    await Promise.resolve()
    expect(settled).toBe(false)
    expect(rejected).toBe(false)

    patchWorkspace({ parentId: 'ws-b' })
    await expect(p).resolves.toBeUndefined()
  })

  it('rejects with ReparentFailedError when the workspace disappears', async () => {
    const wait = watchReparent('ws-fork', 'ws-b')
    const p = wait()

    useSidebarStore.setState({ repos: [] })

    await expect(p).rejects.toThrow('workspace is gone')
  })

  it('rejects with ReparentTimeoutError when nothing lands before the deadline', async () => {
    vi.useFakeTimers()
    const wait = watchReparent('ws-fork', 'ws-b', 1000)
    const p = wait()
    const assertion = expect(p).rejects.toBeInstanceOf(ReparentTimeoutError)

    await vi.advanceTimersByTimeAsync(1000)

    await assertion
  })

  it('unsubscribes once settled — a later store change is inert, not a second settle', async () => {
    const wait = watchReparent('ws-fork', 'ws-b')
    const p = wait()

    patchWorkspace({ parentId: 'ws-b' })
    await p

    expect(() => patchWorkspace({ parentId: 'ws-c' })).not.toThrow()
  })
})
