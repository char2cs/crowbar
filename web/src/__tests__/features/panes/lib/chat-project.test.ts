// §3.5 of the project-scoped panes design: "a chat resolves to exactly one
// project, and cannot resolve to two" — chat → workspace → repo → project,
// with project home resolving through the project's own home workspace. This
// is the derivation the pane store deliberately never performs; it runs when
// a record is minted and once per drop, never in a render path.
import { describe, expect, it, beforeEach, vi } from 'vitest'

const homeWorkspaces: Record<string, string> = {}

vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId: (projectId: string) => homeWorkspaces[projectId] ?? null,
  getHomeOwningChatId: () => null,
}))

import { resolveChatProjectId, resolveWorkspaceProjectId } from '@/features/panes/lib/chat-project'
import { useSidebarStore, type Chat, type Repo, type Workspace } from '@/lib/store/sidebar'
import { useHomeTreeStore } from '@/lib/store/home-tree'

const workspace = (id: string): Workspace => ({ id, branch: id, age: '1m' })

const chat = (id: string, workspaceId: string): Chat => ({
  id,
  repoId: 'repo-1',
  workspaceId,
  title: id,
  order: 0,
})

const repo = (over: Partial<Repo>): Repo => ({
  id: 'repo-1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: '#000',
  workspaces: [],
  ...over,
})

beforeEach(() => {
  for (const key of Object.keys(homeWorkspaces)) delete homeWorkspaces[key]
  useSidebarStore.setState({ repos: [] })
  useHomeTreeStore.setState({ trees: {} })
})

describe('resolveWorkspaceProjectId', () => {
  it("finds a fork's project through its repo", () => {
    useSidebarStore.setState({
      repos: [
        repo({
          id: 'repo-1',
          projectId: 'project-a',
          workspaces: [workspace('ws-1')],
        }),
      ],
    })

    expect(resolveWorkspaceProjectId('ws-1')).toBe('project-a')
  })

  it("finds the repo-home workspace's project too", () => {
    useSidebarStore.setState({
      repos: [repo({ id: 'repo-1', projectId: 'project-a', defaultWorkspaceId: 'ws-main' })],
    })

    expect(resolveWorkspaceProjectId('ws-main')).toBe('project-a')
  })

  it('finds a PROJECT HOME workspace, which rides no repo at all', () => {
    homeWorkspaces['project-b'] = 'ws-home-b'
    useHomeTreeStore.setState({ trees: { 'project-b': { chats: [], folders: [] } } })

    expect(resolveWorkspaceProjectId('ws-home-b')).toBe('project-b')
  })

  it('answers null rather than guessing for a workspace nothing knows', () => {
    expect(resolveWorkspaceProjectId('ws-unknown')).toBeNull()
  })
})

describe('resolveChatProjectId', () => {
  it("reads a repo chat's project straight off the repo that lists it", () => {
    useSidebarStore.setState({
      repos: [
        repo({
          id: 'repo-1',
          projectId: 'project-a',
          chats: [chat('chat-1', 'ws-1')],
        }),
      ],
    })

    expect(resolveChatProjectId('chat-1')).toBe('project-a')
  })

  it('resolves a PROJECT-HOME chat through the home tree, never through repos', () => {
    homeWorkspaces['project-b'] = 'ws-home-b'
    useHomeTreeStore.setState({
      trees: {
        'project-b': { chats: [chat('chat-home', 'ws-home-b')], folders: [] },
      },
    })

    expect(resolveChatProjectId('chat-home')).toBe('project-b')
  })

  it("falls back to the row's own workspace hint when no list names the chat", () => {
    useSidebarStore.setState({
      repos: [
        repo({
          id: 'repo-1',
          projectId: 'project-a',
          workspaces: [workspace('ws-1')],
        }),
      ],
    })

    expect(resolveChatProjectId('chat-unlisted', 'ws-1')).toBe('project-a')
  })

  it('answers null when nothing loaded can name a project', () => {
    expect(resolveChatProjectId('chat-nowhere')).toBeNull()
  })
})
