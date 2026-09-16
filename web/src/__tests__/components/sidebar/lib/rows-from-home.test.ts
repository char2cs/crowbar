import { describe, expect, it } from 'vitest'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { Chat, Folder } from '@/lib/store/sidebar'

const HOME_WS_ID = 'home-ws-1'
/** The chat every fixture's home workspace is owned by — mirrors
 *  rows-from-repo.test.ts's own HOME_ROW_ID: minted chat-first at creation
 *  (never a boot backfill, never retyped to `'branch'` — Task 9), and
 *  `rowsFromHome`'s caller never builds rows before that seed has landed.
 *  Never itself drawn as a row. */
const HOME_ROW_ID = 'home-branch-row'

function makeTestFolder(over: Partial<Folder> & { id: string; name: string }): Folder {
  return { repoId: '', order: 0, ...over }
}

function makeTestChat(over: Partial<Chat> & { id: string; title: string }): Chat {
  return { repoId: '', order: 0, ...over }
}

function homeOwningChat(): Chat {
  return makeTestChat({ id: HOME_ROW_ID, title: '', workspaceId: HOME_WS_ID, ownsWorktree: true })
}

describe('rowsFromHome', () => {
  // Explicit user correction: a first version drew a container "Home" row for
  // every home chat/folder to nest under, mirroring a repo's own home row —
  // rejected outright. Home's chats/folders are FLAT TOP-LEVEL rows, exactly
  // like a repo itself, never one level deeper inside a synthetic parent.
  it('draws no row for the owning branch chat itself', () => {
    const rows = rowsFromHome(HOME_WS_ID, [homeOwningChat()])
    expect(rows).toHaveLength(0)
    expect(rows.find((r) => r.id === HOME_ROW_ID)).toBeUndefined()
  })

  // Task 9: the owning chat is minted chat-first, atomically, at creation —
  // there is no boot backfill left to race, so a caller catching this window
  // (its own creation landed, its chat/folder tree's first seed has not)
  // degrades gracefully instead of throwing: no rows, not a crash.
  it('degrades to no rows, never throws, while the owning chat has not seeded yet', () => {
    expect(() => rowsFromHome(HOME_WS_ID, [])).not.toThrow()
    expect(rowsFromHome(HOME_WS_ID, [])).toEqual([])
  })

  it('a thread on the home workspace becomes a top-level chat-kind row, parentId null', () => {
    const thread = makeTestChat({ id: 'c-1', title: 'Fix the thing', workspaceId: HOME_WS_ID })
    const rows = rowsFromHome(HOME_WS_ID, [homeOwningChat(), thread])
    const row = rows.find((r) => r.id === 'c-1')
    expect(row?.kind).toBe('chat')
    expect(row?.parentId).toBeNull()
    expect(row?.ownsWorktree).toBe(false)
  })

  it('an untitled thread falls back to the shared placeholder label', () => {
    const thread = makeTestChat({ id: 'c-1', title: '', workspaceId: HOME_WS_ID })
    const rows = rowsFromHome(HOME_WS_ID, [homeOwningChat(), thread])
    expect(rows.find((r) => r.id === 'c-1')?.label).toBe(UNTITLED_CHAT_LABEL)
  })

  it('a folder becomes a top-level folder-kind row, parentId null', () => {
    const folder = makeTestFolder({ id: 'f-1', name: 'Notes' })
    const rows = rowsFromHome(HOME_WS_ID, [homeOwningChat()], [folder])
    const row = rows.find((r) => r.id === 'f-1')
    expect(row?.kind).toBe('folder')
    expect(row?.parentId).toBeNull()
  })

  // Project home has no repo, so no folder inside it ever owns a worktree —
  // unlike rows-from-repo.ts's own folder, which always does. Its own "+"
  // must never offer Fork: there is no worktree for it to clone.
  it('a home folder never owns a worktree, so its own "+" cannot fork', () => {
    const folder = makeTestFolder({ id: 'f-1', name: 'Notes' })
    const rows = rowsFromHome(HOME_WS_ID, [homeOwningChat()], [folder])
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
    const rows = rowsFromHome(HOME_WS_ID, [homeOwningChat(), thread], [folder])
    expect(rows.find((r) => r.id === 'c-1')?.parentId).toBe('f-1')
  })

  it('defaults to no chats/folders — just the (empty) owning-chat exclusion', () => {
    const rows = rowsFromHome(HOME_WS_ID, [homeOwningChat()])
    expect(rows).toHaveLength(0)
  })

  // Task 3 put a repo's `order`/`folderId` on its own `Node` row, computed
  // server-side against these SAME real chat/folder siblings — so a chat's
  // rendered `order` now has to carry its REAL wire value through, not a
  // 0..n-1 index compacted from whichever chats/folders happened to reach
  // this one call (which never includes a repo — see this file's own doc).
  // A local index and the real value only ever coincide when nothing here
  // has a gap, so a gap is what proves the real value survives.
  it("a chat's rendered order is its real wire value, not a index compacted from array position", () => {
    const chatA = makeTestChat({ id: 'c-a', title: 'a', workspaceId: HOME_WS_ID, order: 0 })
    const chatB = makeTestChat({ id: 'c-b', title: 'b', workspaceId: HOME_WS_ID, order: 5 })
    const rows = rowsFromHome(HOME_WS_ID, [homeOwningChat(), chatA, chatB])
    expect(rows.find((r) => r.id === 'c-a')?.order).toBe(0)
    expect(rows.find((r) => r.id === 'c-b')?.order).toBe(5)
  })

  // Task 9: `owningChatOf` files a NEW chat/folder directly under its parent
  // workspace's own id — no owning-chat lookup — the instant that parent's
  // `Node{Kind:workspace}` row exists, which project home always has. Such a
  // row must still land at the top level, exactly where every other home
  // chat/folder already does (this file's own doc: no container row here).
  it('a chat parented onto the raw home-workspace id still renders top-level', () => {
    const child = makeTestChat({ id: 'c-1', title: 'New-style child', parentId: HOME_WS_ID })
    const rows = rowsFromHome(HOME_WS_ID, [homeOwningChat(), child])
    const row = rows.find((r) => r.id === 'c-1')
    expect(row?.kind).toBe('chat')
    expect(row?.parentId).toBeNull()
  })
})
