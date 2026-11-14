import { describe, it, expect, afterEach, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
} from '@/features/workspace/stores/workspace-store-registry'
import {
  useOpenWorkspaceCount,
  useWorkingChatCount,
} from '@/features/workspace/stores/hooks/use-open-workspace-counts'
import { forgetEntities } from '@/core/entities/entity-store'
import { resetChatStore, setChatWorking } from '@/__tests__/__fixtures__/agent-chat'

// Mock the IDB-backed persistence so a real destroyWorkspaceStore call (which
// dynamically imports window-pane-store.ts for its buffer-scoped teardown)
// doesn't need a real IndexedDB write path.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
// Fake Monaco adapters so armEditor() (dynamically imported by both
// WorkspaceStore itself and destroyWorkspaceStore's teardown) never touches
// real monaco-editor — same fakes editor-pane-workspace-scope.test.tsx uses.
vi.mock('@/features/editor/lib/monaco-adapters', () => ({
  EDITOR_CREATE_OPTIONS: {},
  langForUri: () => 'plaintext',
  realModelApi: () => ({
    createModel: (value: string, _lang: string, uri: string) => ({
      uri,
      dispose: () => {},
      getValue: () => value,
      setValueIfChanged: () => {},
    }),
    getModel: () => null,
  }),
  realEditorApi: () => ({
    create: () => ({
      setModel: () => {},
      getModel: () => null,
      saveViewState: () => null,
      restoreViewState: () => {},
      layout: () => {},
      dispose: () => {},
      raw: () => null,
    }),
  }),
}))

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  resetChatStore()
})

describe('useOpenWorkspaceCount', () => {
  it('follows the registry as workspaces are opened and destroyed', () => {
    const { result } = renderHook(() => useOpenWorkspaceCount())
    expect(result.current).toBe(0)

    act(() => {
      getOrCreateWorkspaceStore('ws-a')
      getOrCreateWorkspaceStore('ws-b')
    })
    expect(result.current).toBe(2)

    act(() => destroyWorkspaceStore('ws-a'))
    expect(result.current).toBe(1)
  })
})

describe('useWorkingChatCount', () => {
  it('counts the chats the daemon reports mid-turn', () => {
    const { result } = renderHook(() => useWorkingChatCount())
    expect(result.current).toBe(0)

    act(() => {
      setChatWorking('chat-1', true)
      setChatWorking('chat-2', true)
      setChatWorking('chat-3', true)
    })
    expect(result.current).toBe(3)

    act(() => setChatWorking('chat-2', false))
    expect(result.current).toBe(2)
  })

  it('drops a chat the store forgets', () => {
    act(() => setChatWorking('chat-1', true))
    const { result } = renderHook(() => useWorkingChatCount())
    expect(result.current).toBe(1)

    act(() => forgetEntities('chat', ['chat-1']))
    expect(result.current).toBe(0)
  })
})
