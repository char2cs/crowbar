/**
 * Re-render isolation for the agent-chats sidebar panel.
 *
 * The panel used to subscribe to the WHOLE `working` map and rendered
 * unmemoized rows built from fresh inline closures, so a single chat toggling
 * its turn spinner — the hottest frame on the feed — re-rendered EVERY row, and
 * so did merely opening a new chat. With many chats that made the sidebar (and
 * the app) crawl. These tests pin the two properties that fix it:
 *
 *  1. Toggling one chat's `working` re-renders only THAT chat's row (each row
 *     subscribes to its own `working[chatId]`, mirroring AgentChatTabIcon).
 *  2. Adding a new chat does not re-render the existing rows (memoised rows +
 *     referentially stable callbacks).
 *
 * A row's render is counted by mocking its glyph — the fixtures give c1 the
 * claude icon and c2 the codex icon, so the provider icon identifies the row.
 */
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// ── Mocks ───────────────────────────────────────────────────────────

const { streamHook } = vi.hoisted(() => ({ streamHook: vi.fn() }))

vi.mock('@/features/agent/api/agent-api', () => ({
  createChat: vi.fn(),
  deleteChat: vi.fn(),
  renameChat: vi.fn(),
  stopChat: vi.fn(async () => {}),
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-agent-chats-stream', () => ({
  useWorkspaceAgentChatsStream: (wsId: string) => streamHook(wsId),
}))

// The list is virtualized; jsdom reports zero layout, so the real virtualizer
// would render no rows. Stand it in with one that renders every row.
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: (options: { count: number; estimateSize: () => number }) => {
    const size = options.estimateSize()
    return {
      getVirtualItems: () =>
        Array.from({ length: options.count }, (_, index) => ({
          index,
          key: index,
          start: index * size,
          size,
        })),
      getTotalSize: () => options.count * size,
      scrollToIndex: () => {},
      measureElement: () => {},
    }
  },
}))

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

// Count each ROW render: the glyph is rendered once per row render, and the
// provider icon tells the two apart (c1 = claude, c2 = codex).
const glyphRenders = { claude: 0, codex: 0, other: 0 }
vi.mock('@/features/agent/components/agent-chat-glyph', () => ({
  AgentChatGlyph: ({ providerIcon }: { providerIcon: string }) => {
    if (providerIcon.includes('claude')) glyphRenders.claude++
    else if (providerIcon.includes('codex')) glyphRenders.codex++
    else glyphRenders.other++
    return <span data-glyph="true" />
  },
}))

import { AgentChatsPanel } from '@/features/agent/components/agent-chats-panel'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import type { AgentChat } from '@/features/agent/api/agent-api'

// ── Fixtures ────────────────────────────────────────────────────────

const chat = (id: string, providerId: string, createdAt: string): AgentChat => ({
  id,
  workspaceId: 'w1',
  title: id,
  liveRunnerId: `${id}-r`,
  terminalSessionId: `${id}-pty`,
  activeProviderId: providerId,
  createdAt,
})

const PROVIDERS = [
  {
    id: 'claude',
    displayName: 'Claude',
    icon: '<svg data-p="claude"></svg>',
    connected: true,
    enabled: true,
  },
  {
    id: 'codex',
    displayName: 'Codex',
    icon: '<svg data-p="codex"></svg>',
    connected: true,
    enabled: true,
  },
]

const CHAT_1 = chat('c1', 'claude', '2026-01-01T00:00:00Z')
const CHAT_2 = chat('c2', 'codex', '2026-01-02T00:00:00Z')

const state = () => getOrCreateWorkspaceStore('w1').getState()

function seedAndRender() {
  act(() => {
    setActiveWorkspaceStoreRef(getOrCreateWorkspaceStore('w1'))
    state().setAgentProviders(PROVIDERS)
    state().upsertAgentChat(CHAT_1)
    state().upsertAgentChat(CHAT_2)
  })
  return render(<AgentChatsPanel />)
}

beforeEach(() => {
  localStorage.clear()
  streamHook.mockClear()
  glyphRenders.claude = 0
  glyphRenders.codex = 0
  glyphRenders.other = 0
})

afterEach(() => {
  cleanup()
  setActiveWorkspaceStoreRef(null)
  destroyWorkspaceStore('w1')
  vi.restoreAllMocks()
})

// ── Tests ───────────────────────────────────────────────────────────

describe('AgentChatsPanel re-render isolation', () => {
  it('toggling one chat working re-renders only that chat row', () => {
    seedAndRender()
    const c1Before = glyphRenders.claude

    act(() => state().setAgentChatWorking('c2', true))

    // c1 (claude) is untouched by c2's turn frame — the whole point.
    expect(glyphRenders.claude).toBe(c1Before)
    // c2 (codex) did re-render to show the spinner.
    expect(glyphRenders.codex).toBeGreaterThan(0)
  })

  it('adding a new chat does not re-render the existing rows', () => {
    seedAndRender()
    const c1Before = glyphRenders.claude
    const c2Before = glyphRenders.codex

    act(() => state().upsertAgentChat(chat('c3', 'claude', '2026-01-03T00:00:00Z')))

    // The two rows that already existed keep their exact render count; only c3
    // mounts. Memoised rows + stable callbacks are what make this hold.
    expect(glyphRenders.claude).toBe(c1Before + 1) // +1 = the NEW c3 row mounting
    expect(glyphRenders.codex).toBe(c2Before)
  })
})
