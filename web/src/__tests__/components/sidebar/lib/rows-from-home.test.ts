import { describe, expect, it } from 'vitest'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { Chat, Folder } from '@/lib/store/sidebar'

const HOME_WS_ID = 'home-ws-1'
/** The `branch`-typed chat every fixture's home workspace is owned by —
 *  mirrors rows-from-repo.test.ts's own HOME_ROW_ID: the daemon backfills
 *  one for every project home, and `rowsFromHome`'s caller never builds rows
 *  before that seed has landed. Never itself drawn as a row. */
const HOME_ROW_ID = 'home-branch-row'

function makeTestFolder(over: Partial<Folder> & { id: string; name: string }): Folder {
  return { repoId: '', order: 0, ...over }
}

function makeTestChat(over: Partial<Chat> & { id: string; title: string }): Chat {
  return { repoId: '', order: 0, ...over }
}

function homeOwningChat(): Chat {
  return makeTestChat({ id: HOME_ROW_ID, title: '', type: 'branch', workspaceId: HOME_WS_ID })
}

describe('rowsFromHome', () => {
  // Explicit user correction: a first version drew a container "Home" row for
  // every home chat/folder to nest under, mirroring a repo's own home row —
  // rejected outright. Home's chats/folders are FLAT TOP-LEVEL rows, exactly
  // like a repo itself, never one level deeper inside a synthetic parent.
  it('draws no row for the owning branch chat itself', () => {
    const { rows } = rowsFromHome(HOME_WS_ID, [homeOwningChat()])
    expect(rows).toHaveLength(0)
    expect(rows.find((r) => r.id === HOME_ROW_ID)).toBeUndefined()
  })

  it('throws if the daemon has not backfilled an owning branch chat yet', () => {
    expect(() => rowsFromHome(HOME_WS_ID, [])).toThrow()
  })

  it('a thread on the home workspace becomes a top-level chat-kind row, parentId null', () => {
    const thread = makeTestChat({ id: 'c-1', title: 'Fix the thing', workspaceId: HOME_WS_ID })
    const { rows } = rowsFromHome(HOME_WS_ID, [homeOwningChat(), thread])
    const row = rows.find((r) => r.id === 'c-1')
    expect(row?.kind).toBe('chat')
    expect(row?.parentId).toBeNull()
    expect(row?.ownsWorktree).toBe(false)
  })

  it('an untitled thread falls back to the shared placeholder label', () => {
    const thread = makeTestChat({ id: 'c-1', title: '', workspaceId: HOME_WS_ID })
    const { rows } = rowsFromHome(HOME_WS_ID, [homeOwningChat(), thread])
    expect(rows.find((r) => r.id === 'c-1')?.label).toBe(UNTITLED_CHAT_LABEL)
  })

  it('a folder becomes a top-level folder-kind row, parentId null', () => {
    const folder = makeTestFolder({ id: 'f-1', name: 'Notes' })
    const { rows } = rowsFromHome(HOME_WS_ID, [homeOwningChat()], [folder])
    const row = rows.find((r) => r.id === 'f-1')
    expect(row?.kind).toBe('folder')
    expect(row?.parentId).toBeNull()
  })

  // Project home has no repo, so no folder inside it ever owns a worktree —
  // unlike rows-from-repo.ts's own folder, which always does. Its own "+"
  // must never offer Fork: there is no worktree for it to clone.
  it('a home folder never owns a worktree, so its own "+" cannot fork', () => {
    const folder = makeTestFolder({ id: 'f-1', name: 'Notes' })
    const { rows } = rowsFromHome(HOME_WS_ID, [homeOwningChat()], [folder])
    expect(rows.find((r) => r.id === 'f-1')?.ownsWorktree).toBe(false)
  })

  it('a thread filed into a folder nests under the folder, not at the top level', () => {
    const folder = makeTestFolder({ id: 'f-1', name: 'Notes' })
    const thread = makeTestChat({
      id: 'c-1',
      title: 'Filed',
      workspaceId: HOME_WS_ID,
      parentId: 'f-1',
    })
    const { rows } = rowsFromHome(HOME_WS_ID, [homeOwningChat(), thread], [folder])
    expect(rows.find((r) => r.id === 'c-1')?.parentId).toBe('f-1')
  })

  it('defaults to no chats/folders — just the (empty) owning-chat exclusion', () => {
    const { rows } = rowsFromHome(HOME_WS_ID, [homeOwningChat()])
    expect(rows).toHaveLength(0)
  })
})

// Caught live: a repo's own row kept its RAW backend `order`, while every
// chat/folder row here had its `order` silently REPLACED by a positional
// index local to this file's own walk, blind to repos entirely. Two
// independently-dense integer sequences sharing one visual level collide the
// moment both exist in it — `roots.sort(byOrder)` in `sidebar-tree.tsx`
// could only ever agree with what a drag promised by coincidence. These pin
// the fix: a repo's TRUE position — computed by giving it a seat in the same
// `buildSidebarTree` sort a chat/folder already goes through — comes back in
// `repoPositions`, keyed by the repo's own row id.
describe('rowsFromHome — repos interleaved among home siblings', () => {
  it('a repo with the lowest order sorts BEFORE a chat, not always after it', () => {
    const chat = makeTestChat({ id: 'c-1', title: 'testing', workspaceId: HOME_WS_ID, order: 1 })
    const { repoPositions } = rowsFromHome(HOME_WS_ID, [homeOwningChat(), chat], [], [
      { id: 'repo-1', folderId: '', order: 0 },
    ])
    expect(repoPositions.get('repo-1')).toEqual({ parentId: null, order: 0 })
  })

  it('a repo with a higher order sorts AFTER a chat, not always before it', () => {
    const chat = makeTestChat({ id: 'c-1', title: 'testing', workspaceId: HOME_WS_ID, order: 0 })
    const { repoPositions } = rowsFromHome(HOME_WS_ID, [homeOwningChat(), chat], [], [
      { id: 'repo-1', folderId: '', order: 5 },
    ])
    expect(repoPositions.get('repo-1')).toEqual({ parentId: null, order: 1 })
  })

  it('a repo filed into a folder is positioned among that folder’s real children', () => {
    const folder = makeTestFolder({ id: 'f-1', name: 'Notes' })
    const chatInFolder = makeTestChat({
      id: 'c-1',
      title: 'inside',
      workspaceId: HOME_WS_ID,
      parentId: 'f-1',
      order: 0,
    })
    const { repoPositions } = rowsFromHome(
      HOME_WS_ID,
      [homeOwningChat(), chatInFolder],
      [folder],
      [{ id: 'repo-1', folderId: 'f-1', order: 1 }],
    )
    expect(repoPositions.get('repo-1')).toEqual({ parentId: 'f-1', order: 1 })
  })

  it('draws no row for a repo stand-in — the repo keeps rowsFromRepo’s own row', () => {
    const { rows } = rowsFromHome(HOME_WS_ID, [homeOwningChat()], [], [
      { id: 'repo-1', folderId: '', order: 0 },
    ])
    expect(rows.find((r) => r.id === 'repo-1')).toBeUndefined()
  })

  it('several repos and chats mixed at root interleave by raw order, not array position', () => {
    const chatA = makeTestChat({ id: 'c-a', title: 'a', workspaceId: HOME_WS_ID, order: 0 })
    const chatB = makeTestChat({ id: 'c-b', title: 'b', workspaceId: HOME_WS_ID, order: 3 })
    const { repoPositions } = rowsFromHome(
      HOME_WS_ID,
      [homeOwningChat(), chatA, chatB],
      [],
      [
        { id: 'repo-1', folderId: '', order: 1 },
        { id: 'repo-2', folderId: '', order: 2 },
      ],
    )
    // True order: c-a(0), repo-1(1), repo-2(2), c-b(3) — each repo's
    // POSITIONAL index (what walkTreeIntoRows assigns every sibling here)
    // must reflect that real interleave, not "all repos last" or "all
    // repos first".
    expect(repoPositions.get('repo-1')?.order).toBe(1)
    expect(repoPositions.get('repo-2')?.order).toBe(2)
  })

  it('with no repos given, behaves exactly as before (empty repoPositions)', () => {
    const { repoPositions } = rowsFromHome(HOME_WS_ID, [homeOwningChat()])
    expect(repoPositions.size).toBe(0)
  })
})
