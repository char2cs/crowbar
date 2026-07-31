/**
 * Waiting out a re-parent.
 *
 * The trap this pins is that the drop has ALREADY painted the new parent
 * optimistically, so "is the workspace under its new parent" is a question the
 * store answers yes to before the daemon has done anything. The wait has to
 * hold out for a frame ABOUT that workspace, which is the only thing that
 * overwrites the paint with the server's answer.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { watchReparent } from '@/components/layout/reparent-settle'

const repo = (workspaces: Repo['workspaces']): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  workspaces,
})

/** Replace `id`'s row, the way a WorkspaceDTO frame does. */
function frame(id: string, patch: Partial<Repo['workspaces'][number]>): void {
  useSidebarStore.setState((s) => ({
    repos: s.repos.map((r) => ({
      ...r,
      workspaces: r.workspaces.map((w) => (w.id === id ? { ...w, ...patch } : w)),
    })),
  }))
}

beforeEach(() => {
  useSidebarStore.setState({
    // As the drop leaves it: `kid` already painted under `a`.
    repos: [
      repo([
        { id: 'a', branch: 'a', age: '' },
        { id: 'kid', branch: 'kid', parentId: 'a', age: '' },
      ]),
    ],
  })
})

describe('landing', () => {
  it('resolves when a frame reports the new parent', async () => {
    const settled = watchReparent('kid', 'a')()

    frame('kid', { parentId: 'a' })

    await expect(settled).resolves.toBeUndefined()
  })

  it('does not resolve on the optimistic paint it started from', async () => {
    const settled = watchReparent('kid', 'a', 20)()

    // An unrelated row moving does not replace `kid`, so nothing about the
    // re-parent has been answered.
    frame('a', { added: 3 })

    await expect(settled).rejects.toThrow(/did not land/)
  })

  it('does not resolve on a frame that still reports the old parent', async () => {
    const settled = watchReparent('kid', 'a', 20)()

    // The daemon's "working" frame carries the pre-rebase parent.
    frame('kid', { parentId: undefined, working: true })

    await expect(settled).rejects.toThrow(/did not land/)
  })

  // A rebase with nothing to replay can broadcast before the 202 gets back.
  it('is already satisfied when the frame beat the request home', async () => {
    const settled = watchReparent('kid', 'a')
    frame('kid', { parentId: 'a' })

    await expect(settled()).resolves.toBeUndefined()
  })
})

describe('failing', () => {
  it('rejects on a new lastError', async () => {
    const settled = watchReparent('kid', 'a')()

    frame('kid', { parentId: undefined, lastError: 'rebase: could not apply' })

    await expect(settled).rejects.toThrow(/could not apply/)
  })

  it('ignores a lastError the row was already carrying', async () => {
    frame('kid', { lastError: 'something older' })
    const settled = watchReparent('kid', 'a', 20)()

    frame('kid', { parentId: undefined, lastError: 'something older' })

    await expect(settled).rejects.toThrow(/did not land/)
  })

  it('rejects when the row goes away under it', async () => {
    const settled = watchReparent('kid', 'a')()

    useSidebarStore.setState((s) => ({
      repos: s.repos.map((r) => ({ ...r, workspaces: r.workspaces.filter((w) => w.id !== 'kid') })),
    }))

    await expect(settled).rejects.toThrow(/is gone/)
  })
})

// Three ways out, and a drag must never leave a live subscription behind.
describe('cleanup', () => {
  const exits = [
    ['success', () => frame('kid', { parentId: 'a' })],
    ['failure', () => frame('kid', { parentId: undefined, lastError: 'boom' })],
    ['timeout', () => {}],
  ] as const

  for (const [name, provoke] of exits) {
    it(`unsubscribes on ${name}`, async () => {
      const unsubscribed = vi.fn()
      const real = useSidebarStore.subscribe.bind(useSidebarStore)
      const spy = vi
        .spyOn(useSidebarStore, 'subscribe')
        .mockImplementation((listener: Parameters<typeof real>[0]) => {
          const off = real(listener)
          return () => {
            unsubscribed()
            off()
          }
        })

      const settled = watchReparent('kid', 'a', 20)()
      spy.mockRestore()

      provoke()
      await settled.catch(() => {})

      expect(unsubscribed).toHaveBeenCalledTimes(1)
    })
  }
})
