import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import {
  useChatWorkspaceId,
  usePaneEditorWorkspaceIds,
  useViewWorkspaceIds,
} from '@/features/panes/hooks/use-chat-workspace-id'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { AgentChat } from '@/features/agent/api/agent-api'
import type { EditorContent } from '@/features/panes/types/pane-content'

const editorTab = (id: string, wsId: string): EditorContent => ({
  id,
  type: 'editor',
  name: id,
  path: id,
  workspaceId: wsId,
  content: '',
  savedContent: '',
  isDirty: false,
  isVirtual: false,
  tokens: [],
})

const chat = (id: string, wsId: string): AgentChat => ({
  id,
  workspaceId: wsId,
  title: id,
  liveRunnerId: '',
  terminalSessionId: '',
  activeProviderId: 'claude',
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
  parentId: '',
})

afterEach(() => {
  cleanup()
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  resetWindowPaneStoreForTests()
})

/**
 * The render-path half of the resolver. A pane MOUNTS before any workspace
 * store has been seeded with the chat it holds, so the interesting property
 * is not the lookup (covered in pane-chat-workspace.test.ts) but that it is
 * asked again once the answer exists.
 */
describe('useChatWorkspaceId', () => {
  it('answers null while nothing knows the chat, then the owner once a store is seeded', () => {
    const { result } = renderHook(() => useChatWorkspaceId('c1'))
    expect(result.current).toBeNull()

    act(() => {
      // A workspace mounting and its chats stream landing — the exact sequence
      // a freshly opened workspace goes through under a pane already showing
      // one of its chats.
      getOrCreateWorkspaceStore('ws-a')
        .getState()
        .seedAgentChats([chat('c1', 'ws-a')])
    })

    expect(result.current).toBe('ws-a')
  })

  it('re-resolves when a store registered AFTER the subscription is seeded', () => {
    getOrCreateWorkspaceStore('ws-a')
    const { result } = renderHook(() => useChatWorkspaceId('c1'))
    expect(result.current).toBeNull()

    act(() => {
      // A brand-new registry entry: the subscription has to pick this store up
      // too, or a chat whose workspace mounts later is never resolved at all.
      getOrCreateWorkspaceStore('ws-b')
        .getState()
        .seedAgentChats([chat('c1', 'ws-b')])
    })

    expect(result.current).toBe('ws-b')
  })

  it('is null for a pane holding no chat, and subscribes to nothing', () => {
    const { result } = renderHook(() => useChatWorkspaceId(null))

    expect(result.current).toBeNull()
  })

  // The hint is what answers BEFORE any store has mounted at all — the gap
  // that left the file explorer stuck on the wrong repo in a merged split
  // (ide-shell.tsx's own doc on `activePaneWorkspaceHint`).
  it('answers from the hint while no store knows the chat yet', () => {
    const { result } = renderHook(() => useChatWorkspaceId('c1', 'ws-hinted'))
    expect(result.current).toBe('ws-hinted')
  })

  it('prefers a seeded store over the hint once one exists', () => {
    const { result, rerender } = renderHook(
      ({ hint }: { hint: string | null }) => useChatWorkspaceId('c1', hint),
      { initialProps: { hint: 'ws-hinted' } },
    )
    expect(result.current).toBe('ws-hinted')

    act(() => {
      getOrCreateWorkspaceStore('ws-real')
        .getState()
        .seedAgentChats([chat('c1', 'ws-real')])
    })
    rerender({ hint: 'ws-hinted' })

    expect(result.current).toBe('ws-real')
  })
})

/**
 * `WorkspaceHost`'s new "in a view" retention test (keep-alive-policy.ts —
 * `workspaceKeepAliveMinutes` and its time-window policy are gone). Real
 * workspace stores and the real `windowPaneStore`: the records Recents
 * renders from.
 */
describe('useViewWorkspaceIds', () => {
  function seedLivePane(chatId: string): void {
    windowPaneStore.getState().paneActions.openChat(chatId)
  }

  it('is empty with no active workspaces at all', () => {
    const { result } = renderHook(() => useViewWorkspaceIds())
    expect(result.current).toEqual([])
  })

  it("includes a chat's owning workspace once it is live in a pane", () => {
    act(() => {
      getOrCreateWorkspaceStore('ws-a')
        .getState()
        .seedAgentChats([chat('c1', 'ws-a')])
    })
    const { result, rerender } = renderHook(() => useViewWorkspaceIds())
    expect(result.current).toEqual([])

    act(() => {
      seedLivePane('c1')
    })
    rerender()

    expect(result.current).toEqual(['ws-a'])
  })

  it('includes the owner of a chat adopted as a background record', () => {
    act(() => {
      getOrCreateWorkspaceStore('ws-a')
        .getState()
        .seedAgentChats([chat('c1', 'ws-a')])
    })
    const { result, rerender } = renderHook(() => useViewWorkspaceIds())

    act(() => {
      windowPaneStore.getState().paneActions.adoptBackgroundChat('c1', 'p1')
    })
    rerender()

    expect(result.current).toEqual(['ws-a'])
  })

  it('a working chat with no record retains nothing', () => {
    act(() => {
      getOrCreateWorkspaceStore('ws-a')
        .getState()
        .seedAgentChats([chat('c1', 'ws-a')])
      getOrCreateWorkspaceStore('ws-a').getState().setAgentChatWorking('c1', true)
    })
    const { result } = renderHook(() => useViewWorkspaceIds())
    expect(result.current).toEqual([])
  })

  it('drops a workspace the instant its last chat leaves every Recents entry (close, not a grace period)', () => {
    act(() => {
      getOrCreateWorkspaceStore('ws-a')
        .getState()
        .seedAgentChats([chat('c1', 'ws-a')])
      seedLivePane('c1')
    })
    const { result, rerender } = renderHook(() => useViewWorkspaceIds())
    expect(result.current).toEqual(['ws-a'])

    // Close the view: its record goes with its last chat.
    act(() => {
      const { paneActions, activePaneId } = windowPaneStore.getState()
      paneActions.closePane(activePaneId)
    })
    rerender()

    expect(result.current).toEqual([])
  })

  it('unions owners across more than one workspace', () => {
    act(() => {
      getOrCreateWorkspaceStore('ws-a')
        .getState()
        .seedAgentChats([chat('c1', 'ws-a')])
      getOrCreateWorkspaceStore('ws-b')
        .getState()
        .seedAgentChats([chat('c2', 'ws-b')])
      seedLivePane('c1')
      seedLivePane('c2')
    })
    const { result, rerender } = renderHook(() => useViewWorkspaceIds())
    rerender()

    expect([...result.current].sort()).toEqual(['ws-a', 'ws-b'])
  })

  // Live-reported regression, and the direct cause of a continuous re-render
  // loop (measured live: IDEShell re-rendering the whole app tree every
  // ~6ms, 0 fps drops after this fix vs. 12+ before): listChats is
  // repo-scoped, so ws-b's own store can ALSO carry a copy of a chat that
  // really belongs to ws-a. Attributing ownership by the ITERATING store's
  // id (instead of the chat's own `workspaceId`) made `owners` depend on
  // registry iteration order — stable most of the time, but the instant that
  // order shifted (a store destroyed and recreated by WorkspaceHost's own
  // retention reconcile, itself fed by this hook's output) the attribution
  // flipped, producing a DIFFERENT viewWsIds string, which re-triggered the
  // very reconcile that shifted the order — a self-sustaining feedback loop
  // with no user interaction involved at all.
  it("attributes a chat to its OWN workspaceId even when a sibling store's repo-wide copy also lists it", () => {
    act(() => {
      getOrCreateWorkspaceStore('ws-a')
        .getState()
        .seedAgentChats([chat('c1', 'ws-a')])
      // ws-b's store also carries a copy of c1 — a repo-scoped listChats
      // leak, not a real ownership claim (c1's own workspaceId still says
      // ws-a).
      getOrCreateWorkspaceStore('ws-b')
        .getState()
        .seedAgentChats([chat('c1', 'ws-a')])
      seedLivePane('c1')
    })
    const { result, rerender } = renderHook(() => useViewWorkspaceIds())
    rerender()

    expect(result.current).toEqual(['ws-a'])
  })
})

// Regression: an editor-only pane (chatId: null, real editorTabIds) names no
// chat at all, so it was invisible to WorkspaceHost's retention set entirely
// — planRetention (keep-alive-policy.ts) could evict a workspace still
// displaying an open file/terminal split the moment its chat (if any)
// dropped out of Recents, destroying the store (and EditorSurface's
// editorManager) out from under the still-visible pane. Live-reported as
// "Editor failed to load. Try closing and reopening this file."
describe('usePaneEditorWorkspaceIds', () => {
  it('names the workspace an editor-only pane (no chat) holds a file for', () => {
    act(() => {
      windowPaneStore.setState((state) => ({
        buffers: [...state.buffers, editorTab('tab-1', 'ws-a')],
      }))
      windowPaneStore.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal', 'tab-1')
    })

    const { result } = renderHook(() => usePaneEditorWorkspaceIds())

    expect(result.current).toEqual(['ws-a'])
  })

  it('unions across every pane and drops a workspace once its tab closes', () => {
    let paneA = ''
    act(() => {
      windowPaneStore.setState((state) => ({
        buffers: [...state.buffers, editorTab('tab-a', 'ws-a'), editorTab('tab-b', 'ws-b')],
      }))
      const { paneActions } = windowPaneStore.getState()
      paneA = paneActions.splitPane(ROOT_PANE_ID, 'horizontal', 'tab-a')!
      paneActions.splitPane(ROOT_PANE_ID, 'vertical', 'tab-b')
    })
    const { result, rerender } = renderHook(() => usePaneEditorWorkspaceIds())
    expect([...result.current].sort()).toEqual(['ws-a', 'ws-b'])

    act(() => {
      windowPaneStore.setState((state) => ({
        panes: {
          ...state.panes,
          [paneA]: { ...state.panes[paneA]!, editorTabIds: [] },
        },
      }))
    })
    rerender()

    expect(result.current).toEqual(['ws-b'])
  })

  it('answers empty when no pane holds any editor tab', () => {
    const { result } = renderHook(() => usePaneEditorWorkspaceIds())
    expect(result.current).toEqual([])
  })
})
