import { describe, expect, it } from 'vitest'
import { repoRows } from '@/__tests__/__fixtures__/tree-rows'
import { chatIconIndex } from '@/components/sidebar/lib/chat-icon-index'
import type { Chat, Repo, Workspace } from '@/core/entities/repo-types'

function makeTestWorkspace(over: Partial<Workspace> & { id: string; branch: string }): Workspace {
  return { age: '', ...over }
}

function makeTestChat(over: Partial<Chat> & { id: string; title: string }): Chat {
  return { repoId: 'r1', order: 0, ...over }
}

const HOME_ROW_ID = 'home-branch-row'

function makeOwnedWorkspace(
  wsOver: Partial<Workspace> & { id: string; branch: string },
  chatOver: Partial<Chat> & { id: string },
): { workspace: Workspace; chat: Chat } {
  const chat = makeTestChat({ title: '', ...chatOver, workspaceId: wsOver.id, ownsWorktree: true })
  const workspace = makeTestWorkspace({ ...wsOver, owningChatId: chat.id })
  return { workspace, chat }
}

function makeTestRepo(over: Partial<Repo> = {}): Repo {
  const repo: Repo = {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...over,
  }
  if (!repo.defaultWorkspaceId) return repo
  const home = makeTestChat({
    id: HOME_ROW_ID,
    title: '',
    workspaceId: repo.defaultWorkspaceId,
    repoId: repo.id,
    ownsWorktree: true,
  })
  return {
    ...repo,
    defaultOwningChatId: repo.defaultOwningChatId ?? HOME_ROW_ID,
    chats: [home, ...(repo.chats ?? [])],
  }
}

describe('chatIconIndex', () => {
  it("agrees with deriveRows's own icon fields for a workspace-owning chat", () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/x', status: 'pr-conflicts' },
      { id: 'branch-chat-1' },
    )
    const repo = makeTestRepo({ workspaces: [workspace], chats: [chat] })

    const treeRow = repoRows(repo).find((r) => r.id === 'branch-chat-1')
    const icon = chatIconIndex([repo]).get('branch-chat-1')

    expect(icon).toEqual({
      kind: treeRow?.kind,
      ownsWorktree: treeRow?.ownsWorktree,
      branchName: treeRow?.branchName,
      added: treeRow?.added,
      deleted: treeRow?.deleted,
      locked: treeRow?.locked,
      status: treeRow?.status,
      isPlaceholder: treeRow?.isPlaceholder,
      needsProvisioning: treeRow?.needsProvisioning,
      heldByPath: treeRow?.heldByPath,
      placeholderReason: treeRow?.placeholderReason,
    })
    expect(icon?.kind).toBe('branch')
    expect(icon?.status).toBe('pr-conflicts')
  })

  it('a chat that owns no workspace is absent from the index', () => {
    const repo = makeTestRepo({ chats: [makeTestChat({ id: 'bubble-1', title: 'Just a chat' })] })
    expect(chatIconIndex([repo]).has('bubble-1')).toBe(false)
  })

  it('a fresh fork whose Workspace record has not landed yet still marks ownership, with no decoration', () => {
    const chat = makeTestChat({
      id: 'fresh-fork-chat',
      title: 'New fork',
      workspaceId: 'ws-not-here-yet',
      ownsWorktree: true,
    })
    const repo = makeTestRepo({ chats: [chat] })
    expect(chatIconIndex([repo]).get('fresh-fork-chat')).toEqual({
      kind: 'branch',
      ownsWorktree: true,
    })
  })
})
