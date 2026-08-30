import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react'
import { NewTabView } from '@/features/panes/components/new-tab-view'
import { orderedChats } from '@/features/workspace/stores/slices/agent-chats-slice'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'

const { chordMap } = vi.hoisted(() => ({ chordMap: { current: {} as Record<string, string> } }))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => chordMap.current,
}))

vi.mock('@tanstack/react-router', () => ({
  useRouterState: () => '/ide/p1/r1/ws-1',
}))

vi.mock('@/components/layout/context-pill-model', () => ({
  deriveContextPillModel: () => ({
    kind: 'workspace',
    repoName: 'crowbar',
    branchName: 'enhancement/restyling',
  }),
}))

interface FakeChat {
  id: string
  title: string
  activeProviderId: string
  createdAt: string
}
interface FakeProvider {
  id: string
  displayName: string
  icon: string
  connected?: boolean
  enabled?: boolean
}

// Everything a mock factory below closes over must be `vi.hoisted` — mocked
// modules (including the real imports at the bottom of this file, which the
// transform hoists too) are linked before ordinary top-level `const`s run.
const {
  agentChats,
  panes,
  activePaneIdRef,
  setActiveAgentChatId,
  setActivePane,
  setPaneChat,
  openContent,
  fakeStore,
  createChat,
  toastSpawnFailure,
} = vi.hoisted(() => {
  const agentChats = {
    current: {
      chats: [] as FakeChat[],
      order: [] as string[],
      working: {} as Record<string, boolean>,
      providers: [] as FakeProvider[],
    },
  }
  const setActiveAgentChatId = vi.fn()
  // `activePaneId` mutates as a side effect of `setActivePane`, mirroring the
  // real store — openAgentChat (the helper shared with the Chats sidebar) runs
  // for REAL against this store rather than being mocked out, and its
  // not-open-anywhere branch reads `st.activePaneId` AFTER the caller's own
  // `setActivePane` call, so the fake must actually move it.
  const activePaneId = { current: '' }
  const setActivePane = vi.fn((id: string) => {
    activePaneId.current = id
  })
  const openContent = vi.fn()
  const setPaneChat = vi.fn()
  // A stable fake store: `getState()` always returns the SAME object holding
  // the SAME spy references, mirroring the real zustand store (a fresh object
  // per call would make call-order assertions across two `getState()` reads
  // impossible to write meaningfully).
  // openAgentChat runs for real against this store, so these tests exercise
  // the actual reveal-vs-open decision. That needs `panes` (a chat's pane is
  // found by scanning `chatId`, not by a buffer lookup any more).
  const panes = { current: {} as Record<string, { id: string; chatId: string | null }> }

  const fakeState = {
    agentChats: agentChats.current,
    workspaceId: 'ws-1',
    setActiveAgentChatId,
    get activePaneId() {
      return activePaneId.current
    },
    get panes() {
      return panes.current
    },
    paneActions: { setActivePane, setPaneChat },
    bufferActions: { openContent },
  }
  const fakeStore = { getState: () => fakeState }
  return {
    agentChats,
    panes,
    activePaneIdRef: activePaneId,
    setActiveAgentChatId,
    setActivePane,
    setPaneChat,
    openContent,
    fakeStore,
    createChat: vi.fn(),
    toastSpawnFailure: vi.fn(),
  }
})

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStoreContext: (sel: (s: unknown) => unknown) =>
    sel({ agentChats: agentChats.current, workspaceId: 'ws-1' }),
  useWorkspaceStore: () => fakeStore,
}))

vi.mock('@/features/agent/api/agent-api', () => ({ createChat }))
vi.mock('@/features/agent/lib/spawn-error', () => ({ toastSpawnFailure }))

const { setActiveTab, repos } = vi.hoisted(() => ({
  setActiveTab: vi.fn(),
  repos: {
    current: [] as Array<{
      id: string
      name: string
      avatarLabel: string
      avatarColor: string
      avatarURL?: string
      workspaces: Array<{ id: string }>
      defaultWorkspaceId?: string
    }>,
  },
}))
// Selector-aware (the component reads `s.repos` through one) — the heading
// resolves the repo's own mark from this array, not from the pill model.
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: Object.assign(
    (sel?: (s: unknown) => unknown) => (sel ? sel({ repos: repos.current }) : undefined),
    { getState: () => ({ setActiveTab }) },
  ),
}))

const makeChats = (n: number, overrides: Partial<FakeChat> = {}): FakeChat[] =>
  Array.from({ length: n }, (_, i) => ({
    id: `c${i}`,
    title: `Chat ${i}`,
    activeProviderId: 'p1',
    createdAt: new Date(2020, 0, i + 1).toISOString(),
    ...overrides,
  }))

beforeEach(() => {
  vi.clearAllMocks()
  chordMap.current = {}
  agentChats.current.chats = []
  agentChats.current.order = []
  agentChats.current.working = {}
  agentChats.current.providers = [
    {
      id: 'p1',
      displayName: 'Claude',
      icon: '<svg data-testid="p1-icon" />',
      connected: true,
      enabled: true,
    },
  ]
  panes.current = {}
  activePaneIdRef.current = ''
  repos.current = [
    {
      id: 'r1',
      name: 'crowbar',
      avatarLabel: 'CR',
      avatarColor: 'bg-red-500',
      workspaces: [{ id: 'ws-1' }],
    },
  ]
})

describe('NewTabView', () => {
  it('renders the three create actions', () => {
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    expect(screen.getByRole('button', { name: /new file/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /new terminal/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /new chat/i })).toBeInTheDocument()
  })

  it('orders the create actions New Chat → New Terminal → New File', () => {
    // Starting a conversation is what this app is opened to do, so it leads;
    // New File is the rarest of the three and goes last. getAllByRole preserves
    // DOM order, and with no chord map seeded each button's text is just the
    // label. Presence alone (the test above) let any order pass.
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    const actions = screen
      .getAllByRole('button')
      .map((b) => b.textContent?.trim() ?? '')
      .filter((t) => t === 'New File' || t === 'New Terminal' || t === 'New Chat')
    expect(actions).toEqual(['New Chat', 'New Terminal', 'New File'])
  })

  // The decoration must not be on the app's startup path. AsciiCrowbar carries a
  // BAKED POINT CLOUD — a 180,000-character base64 literal — and importing it
  // statically from here put it in the entry shell chunk, where every cold start
  // paid ~108 KB gzip for a backdrop only ever drawn on a New Tab pane. It is a
  // pointer-events-none, aria-hidden backdrop, so a null Suspense fallback costs
  // nothing and shifts no layout (editor-pane.tsx lazies Plate the same way).
  it('loads the ASCII backdrop lazily — it is absent on the first paint', async () => {
    const { container } = render(<NewTabView paneId={ROOT_PANE_ID} />)

    expect(container.querySelector('pre')).toBeNull()
    await waitFor(() => expect(container.querySelector('pre')).not.toBeNull())
  })

  it('shows chord badges resolved from the keymap, NOT hardcoded defaults', () => {
    // The whole reason these go through the registry: rebinding must move the badge.
    chordMap.current = { 'tabs.newTerminal': 'mod+shift+9' }
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    expect(screen.getByText('⌘⇧9')).toBeInTheDocument()
    expect(screen.queryByText('⌘J')).not.toBeInTheDocument()
  })

  // ── The heading carries the repo's own mark ─────────────────────────
  // The pill model only exposes `repoAvatar` for the repo-home workspace, so a
  // heading that read the mark off the model would show nothing on every branch
  // workspace — which is every workspace you actually work in.
  describe('repo mark', () => {
    it("draws the owning repo's mark ahead of the repo/branch stack", () => {
      render(<NewTabView paneId={ROOT_PANE_ID} />)
      const mark = screen.getByText('CR')
      const eyebrow = screen.getByText('crowbar')
      expect(mark).toBeInTheDocument()
      // DOCUMENT_POSITION_FOLLOWING: the mark comes first in reading order.
      expect(mark.compareDocumentPosition(eyebrow) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    })

    it('finds the repo by its DEFAULT workspace too (repo home is not a tree row)', () => {
      repos.current = [
        {
          id: 'r1',
          name: 'crowbar',
          avatarLabel: 'CR',
          avatarColor: 'bg-red-500',
          workspaces: [],
          defaultWorkspaceId: 'ws-1',
        },
      ]
      render(<NewTabView paneId={ROOT_PANE_ID} />)
      expect(screen.getByText('CR')).toBeInTheDocument()
    })

    it('renders the bare heading when no repo row matches, not an empty box', () => {
      repos.current = []
      render(<NewTabView paneId={ROOT_PANE_ID} />)
      expect(screen.queryByText('CR')).not.toBeInTheDocument()
      expect(screen.getByText('enhancement/restyling')).toBeInTheDocument()
    })
  })

  it('caps history at three rows and hands the rest off', () => {
    agentChats.current.chats = makeChats(24)
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    expect(screen.getAllByTestId('nt-chat-row')).toHaveLength(3)
    expect(screen.getByText(/21 more in this worktree/i)).toBeInTheDocument()
  })

  it('renders no hand-off row when everything fits', () => {
    agentChats.current.chats = makeChats(2)
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    expect(screen.getAllByTestId('nt-chat-row')).toHaveLength(2)
    expect(screen.queryByText(/more in this worktree/i)).not.toBeInTheDocument()
  })

  // The old fixture set a `providerIcon` field the component never reads — the
  // real component resolves each row's glyph through `agentChats.providers`
  // (agent-chats-panel.tsx / agent-chat-tab-icon.tsx do the same lookup). This
  // proves that lookup, not the dead field.
  it("resolves each recent chat row's glyph through the providers map", () => {
    agentChats.current.chats = makeChats(1)
    agentChats.current.providers = [
      { id: 'p1', displayName: 'Claude', icon: '<svg data-testid="p1-icon" />' },
    ]
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    expect(screen.getByTestId('p1-icon')).toBeInTheDocument()
  })

  // A chat with no title used to render a blank row — just the glyph, no text.
  it('shows the Untitled fallback for a chat with an empty title', () => {
    agentChats.current.chats = [
      { id: 'c0', title: '', activeProviderId: 'p1', createdAt: '2020-01-01' },
    ]
    render(<NewTabView paneId={ROOT_PANE_ID} />)
    // Same word every surface uses for the untitled state (chat-label.ts).
    expect(screen.getByText('Untitled chat')).toBeInTheDocument()
  })

  // ── Clicking a named chat row OPENS that chat ───────────────────────
  // It used to only switch the sidebar to the Chats tab, which is what the
  // "N more in this worktree" hand-off row does — so a row naming one specific
  // chat behaved identically to the row that means "show me all of them".
  describe('recent chat rows', () => {
    it('opens the chat rather than switching the sidebar', () => {
      agentChats.current.chats = makeChats(1)
      render(<NewTabView paneId={ROOT_PANE_ID} />)

      fireEvent.click(screen.getByTestId('nt-chat-row'))

      // Opening a chat is no longer "adding a tab" — it sets the pane's own
      // chat slot directly (no runner known yet).
      expect(setPaneChat).toHaveBeenCalledWith(ROOT_PANE_ID, 'c0', null)
      expect(openContent).not.toHaveBeenCalled()
      expect(setActiveTab).not.toHaveBeenCalled()
    })

    it('focuses THIS pane first, so the chat lands here and not in the active pane', () => {
      agentChats.current.chats = makeChats(1)
      render(<NewTabView paneId={BOTTOM_PANE_ID} />)

      fireEvent.click(screen.getByTestId('nt-chat-row'))

      expect(setActivePane).toHaveBeenCalledWith(BOTTOM_PANE_ID)
    })

    it('REVEALS a chat already open elsewhere instead of yanking its pane over', () => {
      // Shared with the sidebar via open-agent-chat.ts: opening a chat that
      // already has a pane elsewhere must take you to it, not move it.
      agentChats.current.chats = makeChats(1)
      panes.current = { 'other-pane': { id: 'other-pane', chatId: 'c0' } }

      render(<NewTabView paneId={ROOT_PANE_ID} />)
      fireEvent.click(screen.getByTestId('nt-chat-row'))

      expect(setActivePane).toHaveBeenCalledWith('other-pane')
      expect(setPaneChat).not.toHaveBeenCalled()
      expect(openContent).not.toHaveBeenCalled()
    })

    it('the hand-off row still goes to the sidebar — it means "show me the rest"', () => {
      agentChats.current.chats = makeChats(24)
      render(<NewTabView paneId={ROOT_PANE_ID} />)

      fireEvent.click(screen.getByText(/21 more in this worktree/i))

      // The unified SidebarTree renders under 'workspaces' now — there is no
      // separate Chats tab to switch to (Task 8).
      expect(setActiveTab).toHaveBeenCalledWith('workspaces')
      expect(openContent).not.toHaveBeenCalled()
    })
  })

  // ── I4: New Chat must actually CREATE a chat ────────────────────────
  describe('New Chat action (I4)', () => {
    it('clicking "New Chat" creates a chat with the first enabled provider and opens it', async () => {
      agentChats.current.providers = [
        { id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: true },
        { id: 'p2', displayName: 'Codex', icon: '', connected: true, enabled: true },
      ]
      let resolveCreate!: (id: string) => void
      createChat.mockReturnValue(
        new Promise<string>((resolve) => {
          resolveCreate = resolve
        }),
      )
      render(<NewTabView paneId={ROOT_PANE_ID} />)

      fireEvent.click(screen.getByRole('button', { name: /new chat/i }))

      // Provider-agnostic: picks the FIRST ENABLED provider, the same one the
      // sidebar's unified "New chat" row opens.
      expect(createChat).toHaveBeenCalledWith('ws-1', 'p1')

      agentChats.current.chats = [
        {
          id: 'chat-9',
          title: 'New conversation',
          activeProviderId: 'p1',
          createdAt: '2020-01-01',
        },
      ]
      await act(async () => {
        resolveCreate('chat-9')
        await Promise.resolve()
      })

      expect(setActiveAgentChatId).toHaveBeenCalledWith('chat-9')
      expect(setPaneChat).toHaveBeenCalledWith(ROOT_PANE_ID, 'chat-9', null)
      expect(openContent).not.toHaveBeenCalled()
    })

    it('picks the first ENABLED provider, skipping a disabled leading one', () => {
      // p1 disabled, p2 enabled → New Chat must open p2, never the disabled p1.
      agentChats.current.providers = [
        { id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: false },
        { id: 'p2', displayName: 'Codex', icon: '', connected: true, enabled: true },
      ]
      createChat.mockResolvedValue('chat-1')
      render(<NewTabView paneId={ROOT_PANE_ID} />)

      fireEvent.click(screen.getByRole('button', { name: /new chat/i }))

      expect(createChat).toHaveBeenCalledWith('ws-1', 'p2')
    })

    it('does nothing when every provider is disabled', () => {
      agentChats.current.providers = [
        { id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: false },
      ]
      render(<NewTabView paneId={ROOT_PANE_ID} />)

      fireEvent.click(screen.getByRole('button', { name: /new chat/i }))

      expect(createChat).not.toHaveBeenCalled()
    })

    // …and SAYS so. A live-looking row wearing a ⌘N badge that silently returns
    // is worse than no row: the user presses it, presses the chord, and neither
    // does anything or explains why.
    it('DISABLES the row when every provider is disabled, and says why', () => {
      agentChats.current.providers = [
        { id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: false },
      ]
      render(<NewTabView paneId={ROOT_PANE_ID} />)

      const row = screen.getByRole('button', { name: /new chat/i })
      expect(row).toBeDisabled()
      // Named as the settings rail names it: Agents (spec §11).
      expect(row.title).toMatch(/settings → agents/i)
    })

    it('leaves the row live while a provider is enabled', () => {
      agentChats.current.providers = [
        { id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: true },
      ]
      render(<NewTabView paneId={ROOT_PANE_ID} />)

      expect(screen.getByRole('button', { name: /new chat/i })).toBeEnabled()
    })

    it('reports a spawn failure via toast instead of swallowing it', async () => {
      agentChats.current.providers = [
        { id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: true },
      ]
      const err = new Error('boom')
      createChat.mockRejectedValue(err)
      render(<NewTabView paneId={ROOT_PANE_ID} />)

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /new chat/i }))
        await Promise.resolve()
        await Promise.resolve()
      })

      expect(toastSpawnFailure).toHaveBeenCalledWith(err, 'Claude', 'start')
    })
  })

  // ── I5: "recent" must agree with the sidebar's own ordering ─────────
  describe('recent chats ordering (I5)', () => {
    it("shows the sidebar's own top three (orderedChats), not the raw array order", () => {
      // Raw array deliberately NOT in createdAt order (mirrors "the server
      // returns them with no ORDER BY") so a naive `chats.slice(0, 3)` and
      // `orderedChats(...).slice(0, 3)` disagree — which is exactly the
      // defect: they must not.
      const raw: FakeChat[] = [
        { id: 'newest', title: 'Newest', activeProviderId: 'p1', createdAt: '2024-01-03' },
        { id: 'oldest', title: 'Oldest', activeProviderId: 'p1', createdAt: '2024-01-01' },
        { id: 'middle', title: 'Middle', activeProviderId: 'p1', createdAt: '2024-01-02' },
        { id: 'extra-a', title: 'Extra A', activeProviderId: 'p1', createdAt: '2024-01-04' },
        { id: 'extra-b', title: 'Extra B', activeProviderId: 'p1', createdAt: '2024-01-05' },
      ]
      agentChats.current.chats = raw
      agentChats.current.order = []

      render(<NewTabView paneId={ROOT_PANE_ID} />)

      const expected = orderedChats(raw as never, []).slice(0, 3)
      const renderedTitles = screen.getAllByTestId('nt-chat-row').map((el) => el.textContent)
      expect(renderedTitles).toEqual(expected.map((c) => c.title))
      // Sanity: the fixture really does disagree with the naive raw slice, so
      // this assertion is not accidentally true for any ordering.
      expect(renderedTitles).not.toEqual(raw.slice(0, 3).map((c) => c.title))
    })

    it('a pinned (order-listed) chat surfaces ahead of unpinned ones, matching the sidebar', () => {
      const raw: FakeChat[] = [
        { id: 'a', title: 'A', activeProviderId: 'p1', createdAt: '2024-01-01' },
        { id: 'b', title: 'B', activeProviderId: 'p1', createdAt: '2024-01-02' },
        { id: 'pinned', title: 'Pinned', activeProviderId: 'p1', createdAt: '2024-01-03' },
      ]
      agentChats.current.chats = raw
      agentChats.current.order = ['pinned']

      render(<NewTabView paneId={ROOT_PANE_ID} />)

      const renderedTitles = screen.getAllByTestId('nt-chat-row').map((el) => el.textContent)
      expect(renderedTitles[0]).toBe('Pinned')
      expect(renderedTitles).toEqual(orderedChats(raw as never, ['pinned']).map((c) => c.title))
    })

    it('lists unpinned chats NEWEST first', () => {
      const raw: FakeChat[] = [
        { id: 'oldest', title: 'Oldest', activeProviderId: 'p1', createdAt: '2024-01-01' },
        { id: 'newest', title: 'Newest', activeProviderId: 'p1', createdAt: '2024-01-03' },
        { id: 'middle', title: 'Middle', activeProviderId: 'p1', createdAt: '2024-01-02' },
      ]
      agentChats.current.chats = raw
      agentChats.current.order = []

      render(<NewTabView paneId={ROOT_PANE_ID} />)

      // Pinned LITERALLY, not via orderedChats. The two tests above assert only
      // that this surface AGREES with orderedChats, which holds for either sort
      // direction — which is how an oldest-first "Recent" list shipped. A list
      // labelled Recent has to lead with the newest chat.
      expect(screen.getAllByTestId('nt-chat-row').map((el) => el.textContent)).toEqual([
        'Newest',
        'Middle',
        'Oldest',
      ])
    })
  })

  // ── I7: actions must target the PANE THE SURFACE IS DRAWN IN, not
  // whatever pane happens to be active ─────────────────────────────────
  describe('pane targeting (I7)', () => {
    it("New Terminal activates this surface's pane before opening content", () => {
      render(<NewTabView paneId={BOTTOM_PANE_ID} />)
      fireEvent.click(screen.getByRole('button', { name: /new terminal/i }))
      expect(setActivePane).toHaveBeenCalledWith(BOTTOM_PANE_ID)
      expect(openContent).toHaveBeenCalledWith({ type: 'terminal' })
      // Order matters: the pane must be focused BEFORE the buffer lands, or
      // openContent (which targets get().activePaneId) still races the OLD
      // active pane.
      const activateOrder = setActivePane.mock.invocationCallOrder[0]
      const openOrder = openContent.mock.invocationCallOrder[0]
      expect(activateOrder).toBeLessThan(openOrder)
    })

    it("New File activates this surface's pane before opening content", () => {
      render(<NewTabView paneId={BOTTOM_PANE_ID} />)
      fireEvent.click(screen.getByRole('button', { name: /new file/i }))
      expect(setActivePane).toHaveBeenCalledWith(BOTTOM_PANE_ID)
      expect(openContent).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'editor', isVirtual: true }),
      )
    })

    it("New Chat activates this surface's pane, not whatever pane is globally active", async () => {
      agentChats.current.providers = [
        { id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: true },
      ]
      createChat.mockResolvedValue('chat-1')
      render(<NewTabView paneId={BOTTOM_PANE_ID} />)

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /new chat/i }))
        await Promise.resolve()
        await Promise.resolve()
      })

      expect(setActivePane).toHaveBeenCalledWith(BOTTOM_PANE_ID)
      // Called again right before the chat lands, guarding the async gap.
      expect(setActivePane.mock.calls.every(([id]) => id === BOTTOM_PANE_ID)).toBe(true)
      expect(setPaneChat).toHaveBeenCalledWith(BOTTOM_PANE_ID, 'chat-1', null)
      expect(openContent).not.toHaveBeenCalled()
    })
  })
})
