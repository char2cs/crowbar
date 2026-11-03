import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { UnreadNotice } from '@/components/sidebar/unread-notice'
import { holdScope, startStream } from '@/core/stream/stream'
import { projectDTO, resetEntities } from '@/__tests__/__fixtures__/sidebar-entities'
import type { RepoDTO } from '@/lib/types'

const socket = vi.hoisted(() => ({
  sent: [] as unknown[],
  deliver: (_data: unknown) => {},
}))

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (data: unknown) => void) => {
      socket.deliver = cb
      return () => {}
    },
    send: (_endpoint: string, data: unknown) => void socket.sent.push(data),
    onDrop: () => () => {},
  },
}))

const repo: RepoDTO = {
  id: 'r1',
  projectId: 'p1',
  name: 'alpha',
  path: '/alpha',
  defaultBranch: 'main',
  avatarLabel: 'A',
  avatarColor: '',
  avatarUrl: '',
  avatarEmoji: '',
  order: 0,
}

/** A fresh projects snapshot: the project and its repo, and what it could not read. */
function snapshot(unread?: string[], repos: RepoDTO[] = [repo]) {
  act(() => {
    socket.deliver({
      t: 'entity',
      scope: 'projects',
      kind: 'project',
      id: 'p1',
      v: 1,
      data: projectDTO({ id: 'p1' }),
    })
    for (const r of repos) {
      socket.deliver({ t: 'entity', scope: 'projects', kind: 'repo', id: r.id, v: 1, data: r })
    }
    socket.deliver({ t: 'synced', scope: 'projects', ...(unread ? { unread } : {}) })
  })
}

beforeAll(() => {
  startStream()
  holdScope('projects')
})

beforeEach(() => {
  vi.spyOn(console, 'warn').mockImplementation(() => {})
  resetEntities()
  socket.deliver({ t: 'ready', epoch: 'e1' })
  socket.sent.length = 0
})

describe('the unread notice', () => {
  it('names a repo and a home a fresh snapshot could not read', () => {
    render(<UnreadNotice projectId="p1" />)
    snapshot(['r1', 'p1'])

    expect(screen.getByRole('alert')).toHaveTextContent("Couldn't load chats in alpha")
    expect(screen.getByRole('alert')).toHaveTextContent("Couldn't load this project's chats")
  })

  it('asks for the snapshot again on Retry, and goes once one reads everything', () => {
    render(<UnreadNotice projectId="p1" />)
    snapshot(['r1'])

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(socket.sent).toEqual([
      { op: 'unsub', scope: 'projects' },
      { op: 'sub', scope: 'projects' },
    ])
    snapshot()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('holds Retry until the snapshot it asked for lands', () => {
    render(<UnreadNotice projectId="p1" />)
    snapshot(['r1'])

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(screen.getByRole('button', { name: 'Retry' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(socket.sent).toHaveLength(2)

    snapshot(['r1'])
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  })

  it('lists two unread repos of the same name, each once', () => {
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {})
    const twin: RepoDTO = { ...repo, id: 'r2', path: '/elsewhere/alpha', order: 1 }
    render(<UnreadNotice projectId="p1" />)
    snapshot(['r1', 'r2'], [repo, twin])

    expect(screen.getAllByText("Couldn't load chats in alpha")).toHaveLength(2)
    expect(errors).not.toHaveBeenCalled()
  })

  it("says nothing of another project's repo", () => {
    render(<UnreadNotice projectId="p2" />)
    snapshot(['r1'])

    expect(screen.queryByRole('alert')).toBeNull()
  })
})
