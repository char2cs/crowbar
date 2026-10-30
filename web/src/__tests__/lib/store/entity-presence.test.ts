import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { success } from '@/lib/loadable'
import { isEntityPresent, usePresentEntities } from '@/lib/store/entity-presence'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useProjectDataStore } from '@/lib/store/projects'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

const repo = {
  id: 'r1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: '#000',
  defaultWorkspaceId: 'home-ws',
  workspaces: [{ id: 'ws1', branch: 'feature/a', age: '' }],
  chats: [{ id: 'c1', repoId: 'r1', title: 'chat', order: 0 }],
} as Repo

beforeEach(() => {
  useSidebarStore.setState(getInitialState())
  useHomeTreeStore.setState({ trees: {} })
  useProjectDataStore.setState({ data: success([]) })
})

describe('isEntityPresent', () => {
  it('a project is present once the project list holds it', () => {
    expect(isEntityPresent({ kind: 'project', id: 'p1' })).toBe(false)
    useProjectDataStore.setState({ data: success([{ id: 'p1', name: 'P' } as Project]) })
    expect(isEntityPresent({ kind: 'project', id: 'p1' })).toBe(true)
  })

  it('a repo is present once the sidebar holds it', () => {
    expect(isEntityPresent({ kind: 'repo', id: 'r1' })).toBe(false)
    useSidebarStore.setState({ repos: [repo] })
    expect(isEntityPresent({ kind: 'repo', id: 'r1' })).toBe(true)
  })

  it("a workspace is present as a repo's workspace or its default", () => {
    useSidebarStore.setState({ repos: [repo] })
    expect(isEntityPresent({ kind: 'workspace', id: 'ws1' })).toBe(true)
    expect(isEntityPresent({ kind: 'workspace', id: 'home-ws' })).toBe(true)
    expect(isEntityPresent({ kind: 'workspace', id: 'ws2' })).toBe(false)
  })

  it("a chat is present in a repo's chats or a project home's", () => {
    useSidebarStore.setState({ repos: [repo] })
    expect(isEntityPresent({ kind: 'chat', id: 'c1' })).toBe(true)
    expect(isEntityPresent({ kind: 'chat', id: 'home-chat' })).toBe(false)
    useHomeTreeStore.setState({
      trees: {
        p1: { chats: [{ id: 'home-chat', repoId: '', title: 'h', order: 0 }], folders: [] },
      },
    })
    expect(isEntityPresent({ kind: 'chat', id: 'home-chat' })).toBe(true)
  })
})

describe('usePresentEntities', () => {
  it('re-reads when a chat it names arrives at a project home', () => {
    const refs = [
      { kind: 'chat', id: 'c1' },
      { kind: 'chat', id: 'home-chat' },
    ] as const
    useSidebarStore.setState({ repos: [repo] })
    const { result } = renderHook(() => usePresentEntities(refs))
    expect([...result.current]).toEqual(['chat:c1'])

    act(() =>
      useHomeTreeStore.setState({
        trees: {
          p1: { chats: [{ id: 'home-chat', repoId: '', title: 'h', order: 0 }], folders: [] },
        },
      }),
    )

    expect([...result.current].sort()).toEqual(['chat:c1', 'chat:home-chat'])
  })
})
