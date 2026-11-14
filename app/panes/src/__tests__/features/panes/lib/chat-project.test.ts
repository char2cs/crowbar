// §3.5 of the project-scoped panes design: "a chat resolves to exactly one
// project, and cannot resolve to two" — chat → workspace → repo → project,
// with project home resolving through the project's own home workspace. This
// is the derivation the pane store deliberately never performs; it runs when
// a record is minted and once per drop, never in a render path.
import { describe, expect, it, beforeEach } from 'vitest'

import { resolveChatProjectId, resolveWorkspaceProjectId } from '@/features/panes/lib/chat-project'
import type { Chat, Repo, Workspace } from '@/core/entities/repo-types'
import {
  resetEntities,
  seedHome,
  seedHomeTree,
  seedRepos,
} from '@/__tests__/__fixtures__/sidebar-entities'

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
  resetEntities()
  seedRepos([])
})

describe('resolveWorkspaceProjectId', () => {
  it("finds a fork's project through its repo", () => {
    seedRepos([
      repo({
        id: 'repo-1',
        projectId: 'project-a',
        workspaces: [workspace('ws-1')],
      }),
    ])

    expect(resolveWorkspaceProjectId('ws-1')).toBe('project-a')
  })

  it("finds the repo-home workspace's project too", () => {
    seedRepos([repo({ id: 'repo-1', projectId: 'project-a', defaultWorkspaceId: 'ws-main' })])

    expect(resolveWorkspaceProjectId('ws-main')).toBe('project-a')
  })

  it('finds a PROJECT HOME workspace, which rides no repo at all', () => {
    seedHome('project-b', 'ws-home-b')

    expect(resolveWorkspaceProjectId('ws-home-b')).toBe('project-b')
  })

  it('answers null rather than guessing for a workspace nothing knows', () => {
    expect(resolveWorkspaceProjectId('ws-unknown')).toBeNull()
  })
})

describe('resolveChatProjectId', () => {
  it("reads a repo chat's project straight off the repo that lists it", () => {
    seedRepos([
      repo({
        id: 'repo-1',
        projectId: 'project-a',
        chats: [chat('chat-1', 'ws-1')],
      }),
    ])

    expect(resolveChatProjectId('chat-1')).toBe('project-a')
  })

  it('resolves a PROJECT-HOME chat off its own entity, never through repos', () => {
    seedHomeTree('project-b', { chats: [chat('chat-home', 'ws-home-b')], folders: [] })

    expect(resolveChatProjectId('chat-home')).toBe('project-b')
  })

  it("falls back to the row's own workspace hint when no list names the chat", () => {
    seedRepos([
      repo({
        id: 'repo-1',
        projectId: 'project-a',
        workspaces: [workspace('ws-1')],
      }),
    ])

    expect(resolveChatProjectId('chat-unlisted', 'ws-1')).toBe('project-a')
  })

  it('answers null when nothing loaded can name a project', () => {
    expect(resolveChatProjectId('chat-nowhere')).toBeNull()
  })
})
