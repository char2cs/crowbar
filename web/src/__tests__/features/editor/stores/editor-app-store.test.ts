import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// filesBase()/filesBaseFor() resolve a workspace's URL through
// workspace-scope.ts, not the heavy registry — see platform.test.ts's own
// comment. Only apiFetch (the real HTTP boundary writeWorkspaceFile/writeFile
// ultimately call) needs mocking.
vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return { ...actual, apiFetch: vi.fn() }
})

// Stubbed to a FIXED 'ws-active', standing in for "whichever workspace the
// route/user is currently looking at" — deliberately different from the
// buffer's own 'ws-owner' below. editor-app-store.ts (post-fix) never reads
// this; it exists so a regression back to the old plain writeFile(path,
// content) (which resolves through this) would visibly target ws-active
// instead of crashing outright on an unset registry pointer, keeping this
// test's failure mode the actual corruption scenario, not just a crash.
vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/workspace/stores/workspace-store-registry')>()
  return { ...actual, getActiveWorkspaceId: () => 'ws-active' }
})

import { apiFetch } from '@/lib/api'
import { useEditorAppStore } from '@/features/editor/stores/editor-app-store'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import {
  setWorkspaceScope,
  recordWorkspaceScope,
  __resetWorkspaceScopesForTest,
} from '@/lib/workspace-scope'
import type { EditorContent } from '@/features/panes/types/pane-content'

const mockFetch = apiFetch as ReturnType<typeof vi.fn>

/** Open a buffer belonging to `workspaceId` and immediately mark it dirty —
 *  openContent always creates a CLEAN buffer (savedContent === content), so
 *  dirtiness has to be forced after the fact. */
function openDirtyBuffer(workspaceId: string, path: string, content: string): string {
  const id = windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    path,
    name: path,
    content,
    workspaceId,
  })
  windowPaneStore.setState((s) => {
    s.buffers = s.buffers.map((b) =>
      b.id === id && b.type === 'editor' ? { ...b, isDirty: true } : b,
    )
    return s
  })
  return id
}

function bufferById(id: string): EditorContent {
  return windowPaneStore.getState().buffers.find((b) => b.id === id) as EditorContent
}

beforeEach(() => {
  vi.clearAllMocks()
  mockFetch.mockResolvedValue({})
  resetWindowPaneStoreForTests()
  // Two DIFFERENT workspaces recorded — ws-owner (the buffer's own) and
  // ws-active (whichever workspace the route/user happens to be looking at
  // right now). setWorkspaceScope (not recordWorkspaceScope) also marks its
  // argument as the scope module's OWN "active" pointer, mirroring what a
  // real workspace-store-registry.setActiveWorkspaceId('ws-active') call
  // would do to filesBase()'s resolution.
  recordWorkspaceScope({
    projectId: 'p1',
    repoId: 'r1',
    wsId: 'ws-owner',
    owningChatId: 'chat-owner',
  })
  setWorkspaceScope({
    projectId: 'p1',
    repoId: 'r1',
    wsId: 'ws-active',
    owningChatId: 'chat-active',
  })
})

afterEach(() => {
  __resetWorkspaceScopesForTest()
})

// Task 26 fix round 2 — Critical 1's own committed regression test (fix
// round 1 shipped this fix with no committed test; a reviewer had to write
// their own repro to prove it). Reproduces the exact bug: a dirty buffer
// belonging to workspace A (ws-owner), workspace B (ws-active) is the one
// currently active/on screen. Before the fix, saveEditorBufferById called
// the plain writeFile(path, content), which resolves its target through
// filesBase() -> getActiveWorkspaceId() — i.e. B, not the buffer's own A —
// silently overwriting B's file with A's content. These MUST fail against
// the pre-fix-round-1 code and pass at HEAD.
describe('editor-app-store — Critical 1: saves target the BUFFER\'s own workspace, not the active one', () => {
  it('handleSave writes to the dirty buffer\'s own workspace', async () => {
    openDirtyBuffer('ws-owner', 'a.ts', 'edited content')

    await useEditorAppStore.getState().actions.handleSave()

    expect(mockFetch).toHaveBeenCalledTimes(1)
    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/chats/chat-owner/')
    expect(url).not.toContain('/chats/chat-active/')
    expect(JSON.parse(init.body as string)).toMatchObject({
      path: 'a.ts',
      content: 'edited content',
    })
  })

  it('saveEditorBufferById (Save As, an untitled buffer) writes to the buffer\'s own workspace', async () => {
    const id = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: 'untitled:Untitled-1',
      name: 'Untitled-1',
      content: 'new file content',
      isVirtual: true,
      workspaceId: 'ws-owner',
    })
    vi.stubGlobal('prompt', vi.fn(() => 'new-name.ts'))

    // saveEditorBufferById itself isn't exported from the module — drive it
    // the same way the rest of the app does, through handleSave (this buffer
    // is on the active pane, exactly as a real Save-As keystroke would find
    // it).
    await useEditorAppStore.getState().actions.handleSave()

    expect(mockFetch).toHaveBeenCalledTimes(1)
    const [url] = mockFetch.mock.calls[0] as [string]
    expect(url).toContain('/chats/chat-owner/')
    expect(url).not.toContain('/chats/chat-active/')
    expect(bufferById(id).path).toBe('new-name.ts')
    vi.unstubAllGlobals()
  })

  it('handleSaveAll writes each dirty buffer to ITS OWN workspace, never the active one', async () => {
    openDirtyBuffer('ws-owner', 'a.ts', 'from ws-owner')
    openDirtyBuffer('ws-active', 'b.ts', 'from ws-active')

    const savedCount = await useEditorAppStore.getState().actions.handleSaveAll()

    expect(savedCount).toBe(2)
    expect(mockFetch).toHaveBeenCalledTimes(2)
    const calls = mockFetch.mock.calls.map(([url, init]) => ({
      url: url as string,
      path: (JSON.parse((init as RequestInit).body as string) as { path: string }).path,
    }))
    // Each buffer's own write landed under ITS OWN worktree — named by the chat
    // that holds it, since no files URL carries a workspace id any more — not
    // cross-wired — the bug this locks in would have sent BOTH writes to
    // whichever workspace was merely active.
    const ownerCall = calls.find((c) => c.path === 'a.ts')
    const activeCall = calls.find((c) => c.path === 'b.ts')
    expect(ownerCall?.url).toContain('/chats/chat-owner/')
    expect(activeCall?.url).toContain('/chats/chat-active/')
  })
})
