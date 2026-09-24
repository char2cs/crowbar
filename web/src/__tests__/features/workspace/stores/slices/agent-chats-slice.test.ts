import { describe, expect, it, beforeEach, vi } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  orderedChats,
  selectEnabledProviders,
} from '@/features/workspace/stores/slices/agent-chats-slice'
import type { WorkspaceState } from '@/features/workspace/stores/workspace-store.types'
import type { AgentChat, AgentChatFolder, AgentProvider } from '@/features/agent/api/agent-api'
import { promptQueueStorageKey } from '@/features/agent/lib/prompt-queue-persistence'
import {
  chatSnapshot,
  setChatTerminalWait,
  setChatWorking,
} from '@/__tests__/__fixtures__/agent-chat'

/** A fresh snapshot of a chat — every call a newer version, like every read
 *  the daemon answers. */
const chat = (id: string, createdAt: string): AgentChat =>
  chatSnapshot({ id, title: id, activeProviderId: 'claude', createdAt })

describe('agent-chats-slice', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('upserts (insert then replace by id) and removes', () => {
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().applyAgentChat(chat('c1', '2026-01-02T00:00:00Z')) // replace
    expect(s.getState().agentChats.chats).toHaveLength(1)
    expect(s.getState().agentChats.chats[0].createdAt).toBe('2026-01-02T00:00:00Z')
    s.getState().removeAgentChat('c1')
    expect(s.getState().agentChats.chats).toHaveLength(0)
  })

  // ── streamingMessages: upsert by id, not one slot ─────────────────────────
  // Regression: this used to be `Record<string, {id,text}>` — a single slot
  // per chat, unconditionally overwritten. Codex can have more than one
  // message item open in a turn; the second item's first delta silently
  // dropped the first item's still-growing text from the live view (it was
  // always safe server-side, so the ledger "reconciled" it back a moment
  // later — but the transcript visibly lost a paragraph until then).

  it('setAgentChatStreamingMessage upserts by id — a second id does not drop the first', () => {
    const s = createWorkspaceStore('w1')

    s.getState().setAgentChatStreamingMessage('c1', { id: 'm1', text: 'first paragraph' })
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm2', text: 'second item' })

    expect(s.getState().agentChats.streamingMessages['c1']).toEqual([
      { id: 'm1', text: 'first paragraph' },
      { id: 'm2', text: 'second item' },
    ])
  })

  it('setAgentChatStreamingMessage replaces the SAME id in place, preserving order', () => {
    const s = createWorkspaceStore('w1')

    s.getState().setAgentChatStreamingMessage('c1', { id: 'm1', text: 'first paragraph' })
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm2', text: 'second' })
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm1', text: 'first paragraph, growing' })

    expect(s.getState().agentChats.streamingMessages['c1']).toEqual([
      { id: 'm1', text: 'first paragraph, growing' },
      { id: 'm2', text: 'second' },
    ])
  })

  it('setAgentChatStreamingMessage(chatId, null) clears every entry for that chat', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm1', text: 'a' })
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm2', text: 'b' })

    s.getState().setAgentChatStreamingMessage('c1', null)

    expect(s.getState().agentChats.streamingMessages['c1']).toBeUndefined()
  })

  it('does not touch another chat entirely — no cross-chat clobbering', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm1', text: 'chat one' })

    s.getState().setAgentChatStreamingMessage('c2', { id: 'm2', text: 'chat two' })

    expect(s.getState().agentChats.streamingMessages['c1']).toEqual([
      { id: 'm1', text: 'chat one' },
    ])
  })

  // ── pruneAgentChatStreamingMessages: the bounded-growth fix ───────────────
  // streamingMessages[chatId] only ever grew: entries land via
  // setAgentChatStreamingMessage, and nothing removed one once its content
  // was durably confirmed in the ledger — a real unbounded-memory-growth
  // issue over a very long chat session. This is the targeted prune the
  // store side needed instead of a blanket clear on a turn boundary (that
  // was tried and reverted: an interrupted runner's stream can legitimately
  // keep growing after a new turn starts, so wiping the whole array there
  // deleted a reply still arriving).

  it('pruneAgentChatStreamingMessages drops only the given ids, keeping the rest', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm1', text: 'first' })
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm2', text: 'second' })
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm3', text: 'third' })

    s.getState().pruneAgentChatStreamingMessages('c1', ['m1', 'm3'])

    expect(s.getState().agentChats.streamingMessages['c1']).toEqual([{ id: 'm2', text: 'second' }])
  })

  it('pruneAgentChatStreamingMessages deletes the chat entry entirely once it empties out', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm1', text: 'first' })

    s.getState().pruneAgentChatStreamingMessages('c1', ['m1'])

    expect(s.getState().agentChats.streamingMessages['c1']).toBeUndefined()
  })

  it('pruneAgentChatStreamingMessages is a no-op for an id that is not present', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm1', text: 'first' })

    s.getState().pruneAgentChatStreamingMessages('c1', ['not-there'])

    expect(s.getState().agentChats.streamingMessages['c1']).toEqual([{ id: 'm1', text: 'first' }])
  })

  it('pruneAgentChatStreamingMessages does not touch another chat', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatStreamingMessage('c1', { id: 'm1', text: 'chat one' })
    s.getState().setAgentChatStreamingMessage('c2', { id: 'm1', text: 'chat two' })

    s.getState().pruneAgentChatStreamingMessages('c1', ['m1'])

    expect(s.getState().agentChats.streamingMessages['c1']).toBeUndefined()
    expect(s.getState().agentChats.streamingMessages['c2']).toEqual([
      { id: 'm1', text: 'chat two' },
    ])
  })

  // Scroll positions themselves are no longer part of this store — see
  // transcript-scroll-positions.test.ts. removeAgentChat's own delegation to
  // that module's clearScrollPosition is covered there, against the module
  // directly, since this slice has no way to observe it.

  // ── excalidrawEditRequests: a one-shot "open the takeover with this scene"
  //    signal from an Edit button buried in a chat's transcript up to the
  //    composer that owns the takeover — cleared once the composer consumes it.

  it('requestExcalidrawEdit records the scene for that chat only', () => {
    const s = createWorkspaceStore('w1')
    const scene = { elements: [{ type: 'rectangle' }], appState: {} }

    s.getState().requestExcalidrawEdit('c1', scene)

    expect(s.getState().agentChats.excalidrawEditRequests['c1']).toEqual(scene)
    expect(s.getState().agentChats.excalidrawEditRequests['c2']).toBeUndefined()
  })

  it('clearExcalidrawEditRequest removes only that chat’s pending request', () => {
    const s = createWorkspaceStore('w1')
    const scene = { elements: [], appState: {} }
    s.getState().requestExcalidrawEdit('c1', scene)
    s.getState().requestExcalidrawEdit('c2', scene)

    s.getState().clearExcalidrawEditRequest('c1')

    expect(s.getState().agentChats.excalidrawEditRequests['c1']).toBeUndefined()
    expect(s.getState().agentChats.excalidrawEditRequests['c2']).toEqual(scene)
  })

  it('removeAgentChat forgets a pending excalidraw edit request', () => {
    const s = createWorkspaceStore('w1')
    s.getState().requestExcalidrawEdit('c1', { elements: [], appState: {} })

    s.getState().removeAgentChat('c1')

    expect(s.getState().agentChats.excalidrawEditRequests['c1']).toBeUndefined()
  })

  // ── The sticky model / effort selection ───────────────────────────────────
  // The PATCH answers 202 with no body and rides no lifecycle frame, so this write
  // is the only thing that brings an accepted pair back into the store.

  it('setAgentChatSelection writes BOTH halves of an accepted selection', () => {
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChat(chat('c1', '2026-01-01T00:00:00Z'))

    s.getState().setAgentChatSelection('c1', 'gpt-5.6-luna', 'max')

    expect(s.getState().agentChats.chats[0]).toMatchObject({
      model: 'gpt-5.6-luna',
      effort: 'max',
    })
  })

  it("setAgentChatSelection stores '' as the cleared half, not as an absent field", () => {
    // '' IS the value that means "the provider's own default" — the same thing the
    // endpoint takes. Deleting the field instead would make a cleared selection
    // indistinguishable from one that was never read.
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().setAgentChatSelection('c1', 'gpt-5.6-sol', 'ultra')

    s.getState().setAgentChatSelection('c1', '', '')

    expect(s.getState().agentChats.chats[0].model).toBe('')
    expect(s.getState().agentChats.chats[0].effort).toBe('')
  })

  it('setAgentChatSelection ignores a chat the store does not hold', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatSelection('ghost', 'sonnet', 'high')
    expect(s.getState().agentChats.chats).toHaveLength(0)
  })

  it('seedAgentChats grounds an omitted working value to idle — a dropped turn_stopped cannot strand a spinner', () => {
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    setChatWorking(s, 'c1', true) // mid-turn when the socket dropped
    expect(s.getState().agentChats.working.c1).toBe(true)

    // The reconnect read is a NEWER snapshot saying idle: it replaces the
    // pre-outage true, which would otherwise stand forever.
    s.getState().seedAgentChats([chat('c1', '2026-01-01T00:00:00Z')])

    expect(s.getState().agentChats.working.c1).toBeUndefined()
    expect(s.getState().agentChats.chats).toHaveLength(1)
  })

  it('seedAgentChats restores server-folded working on initial load and reconnect', () => {
    const s = createWorkspaceStore('w1')
    setChatWorking(s, 'stale', true)

    const vanished = s.getState().seedAgentChats([
      { ...chat('busy', '2026-01-01T00:00:00Z'), working: true },
      { ...chat('idle', '2026-01-02T00:00:00Z'), working: false },
    ])

    expect(s.getState().agentChats.working.busy).toBe(true)
    expect(s.getState().agentChats.working.idle).toBeUndefined()
    // A chat the list does not mention is a SUSPECT the caller confirms, never
    // a row the seed deletes on its own say-so.
    expect(vanished).toEqual(['stale'])
  })

  // ── seedAgentChats: reconciling a stuck `compacting` flag ──────────────────
  // Regression: `compacting` is driven ENTIRELY by the live compaction_started/
  // compaction_stopped WS frames (there is no ledger record to fall back on —
  // see AgentChatsState.compacting's own doc comment), and unlike `working` and
  // `terminalWaits` it was never reconciled by this reseed at all. If the
  // compaction_stopped edge — and the local 60s backstop timer racing it — are
  // BOTH lost (the app is closed for the whole compaction window, then reopened),
  // nothing ever clears it: reported live as "/compact finished, closed the chat
  // mid-compaction, reopened it, and Crowbar still thinks it's compacting."
  // A chat the fresh GET reports as NOT working cannot possibly still be
  // compacting (compaction keeps the chat busy the same as any other turn), so
  // that is the same proof the live self-heal already trusts for any other
  // frame, applied here to the GET this reseed IS reading.

  it("seedAgentChats clears a stuck `compacting` flag once the authoritative reseed shows the chat isn't busy — the missed compact_post/closed-app case", () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatCompacting('c1', true)

    s.getState().seedAgentChats([{ ...chat('c1', '2026-01-01T00:00:00Z'), working: false }])

    expect(s.getState().agentChats.compacting.c1).toBeUndefined()
  })

  it('seedAgentChats leaves `compacting` alone while the fresh reseed still shows the chat busy', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatCompacting('c1', true)

    s.getState().seedAgentChats([{ ...chat('c1', '2026-01-01T00:00:00Z'), working: true }])

    expect(s.getState().agentChats.compacting.c1).toBe(true)
  })

  it('notifyAgentChatMessages advances every current chat revision independently of reconnect GETs', () => {
    const s = createWorkspaceStore('w1')
    s.getState().seedAgentChats([
      { ...chat('c1', '2026-01-01T00:00:00Z'), working: false },
      { ...chat('c2', '2026-01-02T00:00:00Z'), working: false },
    ])

    expect(s.getState().agentChats.turnRevision).toEqual({})

    // An idle -> idle turn can complete while the socket is down, so folded
    // `working` alone cannot reveal it. Reconnect explicitly invalidates every
    // surviving chat's message page, even when the folded state did not change.
    s.getState().notifyAgentChatMessages()

    expect(s.getState().agentChats.turnRevision).toEqual({ c1: 1, c2: 1 })

    s.getState().notifyAgentChatMessages()
    expect(s.getState().agentChats.turnRevision).toEqual({ c1: 2, c2: 2 })
  })

  it('seedAgentChats REPORTS chats absent from the response instead of dropping them', () => {
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().applyAgentChat(chat('c2', '2026-01-02T00:00:00Z'))

    // A repo-scoped list can omit a project-home chat that still exists, and a
    // chat created after the list was served is newer than the list: the seed
    // names the absentees for the stream to confirm (a not-found forgets them).
    const vanished = s.getState().seedAgentChats([chat('c1', '2026-01-01T00:00:00Z')])

    expect(vanished).toEqual(['c2'])
    expect(
      s
        .getState()
        .agentChats.chats.map((c) => c.id)
        .sort(),
    ).toEqual(['c1', 'c2'])
  })

  // THE RULE (invariant A6): a snapshot applies only if its version is newer.
  // An older read landing last — whichever caller issued it — cannot overwrite
  // what a frame already delivered.
  it('an older snapshot never overwrites a newer one, whatever order they land in', () => {
    const s = createWorkspaceStore('w1')
    const older = { ...chat('c1', '2026-01-01T00:00:00Z'), liveRunnerId: '' }
    const newer = { ...chat('c1', '2026-01-01T00:00:00Z'), liveRunnerId: 'r1' }

    expect(s.getState().applyAgentChat(newer).kind).toBe('applied')
    expect(s.getState().applyAgentChat(older).kind).toBe('stale')
    s.getState().seedAgentChats([older])

    expect(s.getState().agentChats.chats[0].liveRunnerId).toBe('r1')
  })

  it('a delete frame removes the chat and every per-chat entry (A7)', () => {
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChat({ ...chat('c1', '2026-01-01T00:00:00Z'), working: true })
    s.getState().setAgentChatTelemetry('c1', { observedAt: '', source: 'callback' })
    s.getState().setAgentChatStreamingPlan('c1', [{ text: 'a', status: 'pending' }])

    const outcome = s.getState().applyAgentChatFrame({ chatId: 'c1', kind: 'deleted' })

    expect(outcome.kind).toBe('deleted')
    const st = s.getState().agentChats
    expect(st.chats).toHaveLength(0)
    expect(st.working.c1).toBeUndefined()
    expect(st.telemetry.c1).toBeUndefined()
    expect(st.streamingPlan.c1).toBeUndefined()
    expect(st.turnRevision.c1).toBeUndefined()
  })

  it('a chat first seen on a frame joins the top of a saved arrangement', () => {
    const s = createWorkspaceStore('w1')
    s.getState().seedAgentChats([chat('a', '2026-01-01T00:00:00Z')])
    s.getState().setAgentChatOrder(['a'])

    s.getState().applyAgentChatFrame({
      chatId: 'b',
      kind: 'created',
      chat: chat('b', '2026-01-02T00:00:00Z'),
    })

    expect(s.getState().agentChats.order).toEqual(['b', 'a'])
  })

  it('seedAgentChats keeps an active id that still exists, and takes the server copy of each chat', () => {
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChat({ ...chat('c1', '2026-01-01T00:00:00Z'), title: 'stale title' })
    s.getState().setActiveAgentChatId('c1')

    s.getState().seedAgentChats([{ ...chat('c1', '2026-01-01T00:00:00Z'), title: 'server title' }])

    expect(s.getState().agentChats.activeChatId).toBe('c1')
    expect(s.getState().agentChats.chats[0].title).toBe('server title')
  })

  it('toggles the working map and stores providers/active id', () => {
    const s = createWorkspaceStore('w1')
    setChatWorking(s, 'c1', true)
    expect(s.getState().agentChats.working.c1).toBe(true)
    expect(s.getState().agentChats.turnRevision.c1).toBe(1)
    setChatWorking(s, 'c1', false)
    expect(s.getState().agentChats.working.c1).toBeUndefined()
    expect(s.getState().agentChats.turnRevision.c1).toBe(2)
    s.getState().setAgentProviders([
      {
        id: 'claude',
        displayName: 'Claude',
        icon: '<svg/>',
        connected: true,
        enabled: true,
        mcpEnabled: true,
        modelSelect: false,
        effortSelect: false,
        compaction: false,
        hasTerminal: true,
        hotswap: false,
        terminalStartHere: false,
      },
    ])
    expect(s.getState().agentChats.providers).toHaveLength(1)
    s.getState().setActiveAgentChatId('c1')
    expect(s.getState().agentChats.activeChatId).toBe('c1')
  })

  // THE REGRESSION, reported live 2026-09-12: a "chat_busy" queued prompt
  // resent itself into a still-generating turn, corrupting its output mid-
  // stream. Root cause: use-prompt-queue.ts treats every turnRevision advance
  // while `working` reads false as an authoritative idle edge and releases
  // the queue's busy barrier — but a periodic reconcile poll re-announcing
  // the SAME value it already had (working stayed true; or a stale false
  // reading it merely re-confirmed) bumped the revision anyway, on a call
  // that carried no real transition at all.
  it('re-announcing the same working value is a no-op — it must not advance turnRevision', () => {
    const s = createWorkspaceStore('w1')
    setChatWorking(s, 'c1', true)
    expect(s.getState().agentChats.turnRevision.c1).toBe(1)

    setChatWorking(s, 'c1', true)
    expect(s.getState().agentChats.working.c1).toBe(true)
    expect(s.getState().agentChats.turnRevision.c1).toBe(1)

    setChatWorking(s, 'c1', false)
    expect(s.getState().agentChats.turnRevision.c1).toBe(2)
    setChatWorking(s, 'c1', false)
    expect(s.getState().agentChats.turnRevision.c1).toBe(2)
  })

  it('working map defaults to idle (undefined) for chats never toggled', () => {
    const s = createWorkspaceStore('w1')
    expect(s.getState().agentChats.working['never-touched']).toBeUndefined()
  })

  it('removeAgentChat clears the working entry, order membership, and active id when it matches', () => {
    const s = createWorkspaceStore('w2')
    s.getState().applyAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    setChatWorking(s, 'c1', true)
    localStorage.setItem(promptQueueStorageKey('w2', 'c1'), 'pending')
    s.getState().setAgentChatOrder(['c1'])
    s.getState().setActiveAgentChatId('c1')

    s.getState().removeAgentChat('c1')

    expect(s.getState().agentChats.chats).toHaveLength(0)
    expect(s.getState().agentChats.working.c1).toBeUndefined()
    expect(s.getState().agentChats.turnRevision.c1).toBeUndefined()
    expect(localStorage.getItem(promptQueueStorageKey('w2', 'c1'))).toBeNull()
    expect(s.getState().agentChats.order).toEqual([])
    expect(s.getState().agentChats.activeChatId).toBeNull()
  })

  it('removeAgentChat leaves an unrelated active id untouched', () => {
    const s = createWorkspaceStore('w3')
    s.getState().applyAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().applyAgentChat(chat('c2', '2026-01-02T00:00:00Z'))
    s.getState().setActiveAgentChatId('c2')

    s.getState().removeAgentChat('c1')

    expect(s.getState().agentChats.activeChatId).toBe('c2')
    expect(s.getState().agentChats.chats.map((c) => c.id)).toEqual(['c2'])
  })

  it('setAgentChatOrder updates state and persists per-workspace to localStorage', () => {
    const s = createWorkspaceStore('w4')
    s.getState().setAgentChatOrder(['b', 'a'])
    expect(s.getState().agentChats.order).toEqual(['b', 'a'])
    expect(localStorage.getItem('crowbar:agent-chat-order:w4')).toBe(JSON.stringify(['b', 'a']))
  })

  it('hydrateAgentChatOrder loads a previously persisted order for the workspace (round trip)', () => {
    const writer = createWorkspaceStore('w5')
    writer.getState().setAgentChatOrder(['x', 'y'])

    // A fresh store for the same workspace starts un-hydrated...
    const reader = createWorkspaceStore('w5')
    expect(reader.getState().agentChats.order).toEqual([])
    // ...until explicitly hydrated, at which point it picks up the persisted order.
    reader.getState().hydrateAgentChatOrder()
    expect(reader.getState().agentChats.order).toEqual(['x', 'y'])
  })

  it('hydrateAgentChatOrder defaults to an empty order when nothing was persisted', () => {
    const s = createWorkspaceStore('w6')
    s.getState().hydrateAgentChatOrder()
    expect(s.getState().agentChats.order).toEqual([])
  })

  it('hydrateAgentChatOrder recovers from corrupt persisted JSON', () => {
    localStorage.setItem('crowbar:agent-chat-order:w7', 'not-json{{{')
    const s = createWorkspaceStore('w7')
    s.getState().hydrateAgentChatOrder()
    expect(s.getState().agentChats.order).toEqual([])
  })

  it('setAgentChatOrder is best-effort when localStorage.setItem throws', () => {
    const s = createWorkspaceStore('w8')
    const spy = vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded')
    })
    expect(() => s.getState().setAgentChatOrder(['a'])).not.toThrow()
    expect(s.getState().agentChats.order).toEqual(['a'])
    spy.mockRestore()
  })

  it('orderedChats: saved order first, unknown chats appended NEWEST first', () => {
    const chats = [
      chat('a', '2026-01-03T00:00:00Z'),
      chat('b', '2026-01-01T00:00:00Z'),
      chat('c', '2026-01-02T00:00:00Z'),
    ]
    // order pins b then a; c is absent → appended after.
    expect(orderedChats(chats, ['b', 'a']).map((x) => x.id)).toEqual(['b', 'a', 'c'])
    // empty order → createdAt DESCENDING. A chat you just started belongs at the
    // top, and the New Tab's "Recent" list takes its top-N straight from here.
    expect(orderedChats(chats, []).map((x) => x.id)).toEqual(['a', 'c', 'b'])
  })

  // A `created` frame RESEEDS the list (use-workspace-agent-chats-stream), so
  // this — not upsert — is where a brand-new chat first appears.
  describe('seedAgentChats: a new chat joins the top of a saved order', () => {
    it('promotes a newly arrived chat above the chats the user arranged', () => {
      const s = createWorkspaceStore('w1')
      s.getState().seedAgentChats([
        chat('a', '2026-01-01T00:00:00Z'),
        chat('b', '2026-01-02T00:00:00Z'),
      ])
      s.getState().setAgentChatOrder(['a', 'b'])

      s.getState().seedAgentChats([
        chat('a', '2026-01-01T00:00:00Z'),
        chat('b', '2026-01-02T00:00:00Z'),
        chat('c', '2026-01-03T00:00:00Z'),
      ])

      // Without this, one drag pins the whole list and every later chat sinks
      // below all of them — off the New Tab's capped Recent list entirely.
      expect(s.getState().agentChats.order).toEqual(['c', 'a', 'b'])
      expect(JSON.parse(localStorage.getItem('crowbar:agent-chat-order:w1') ?? '[]')).toEqual([
        'c',
        'a',
        'b',
      ])
    })

    it('does not start an order for a user who has never arranged the list', () => {
      const s = createWorkspaceStore('w1')
      s.getState().seedAgentChats([chat('a', '2026-01-01T00:00:00Z')])
      s.getState().seedAgentChats([
        chat('a', '2026-01-01T00:00:00Z'),
        chat('b', '2026-01-02T00:00:00Z'),
      ])

      // With no saved arrangement the newest-first sort already puts b first;
      // writing an order here would start pinning a list nobody arranged.
      expect(s.getState().agentChats.order).toEqual([])
    })

    it('the first seed of a session never rewrites the saved order', () => {
      const s = createWorkspaceStore('w1')
      s.getState().setAgentChatOrder(['b', 'a'])

      // Every chat looks "new" against an empty previous list — promoting them
      // would scramble exactly the arrangement being restored.
      s.getState().seedAgentChats([
        chat('a', '2026-01-01T00:00:00Z'),
        chat('b', '2026-01-02T00:00:00Z'),
      ])

      expect(s.getState().agentChats.order).toEqual(['b', 'a'])
    })
  })

  it('orderedChats ignores order entries that no longer correspond to a chat', () => {
    const chats = [chat('a', '2026-01-01T00:00:00Z')]
    expect(orderedChats(chats, ['ghost', 'a']).map((x) => x.id)).toEqual(['a'])
  })

  // The one selector every New-chat surface leans on: the enabled subset, in the
  // backend's priority order. A disabled provider must be dropped (spec §2.2 —
  // "disabled = hidden entirely"), and the surviving order preserved unchanged.
  it('selectEnabledProviders drops disabled providers, preserving order', () => {
    const provider = (id: string, enabled: boolean): AgentProvider => ({
      id,
      displayName: id,
      icon: '',
      connected: true,
      enabled,
      mcpEnabled: true,
      modelSelect: false,
      effortSelect: false,
      compaction: false,
      hasTerminal: true,
      hotswap: false,
      terminalStartHere: false,
    })
    const s = {
      agentChats: {
        providers: [provider('codex', true), provider('claude', false), provider('gemini', true)],
      },
    } as unknown as WorkspaceState
    expect(selectEnabledProviders(s).map((p) => p.id)).toEqual(['codex', 'gemini'])
  })

  it('selectEnabledProviders returns [] when nothing is enabled', () => {
    const s = {
      agentChats: {
        providers: [
          {
            id: 'codex',
            displayName: 'Codex',
            icon: '',
            connected: true,
            enabled: false,
            mcpEnabled: true,
          },
        ],
      },
    } as unknown as WorkspaceState
    expect(selectEnabledProviders(s)).toEqual([])
  })
})

// ── folders: the chats-tree grouping rows chat-tree-commit.ts writes through ──
// Beside the chats rather than inside them (see AgentChatsState.folders) — a
// folder is a peer of a chat, and the two interleave on one shared `order`.

const folder = (id: string, parentId: string, order: number, name = id): AgentChatFolder => ({
  id,
  workspaceId: 'w1',
  parentId,
  name,
  order,
})

describe('agent-chats-slice: folders', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('starts with an empty folder list', () => {
    const s = createWorkspaceStore('w1')
    expect(s.getState().agentChats.folders).toEqual([])
  })

  it('seedAgentChatFolders REPLACES the whole list — an authoritative GET, not a merge', () => {
    const s = createWorkspaceStore('w1')
    s.getState().seedAgentChatFolders([folder('f1', '', 0)])
    s.getState().seedAgentChatFolders([folder('f2', '', 0)]) // f1 must be dropped, not kept
    expect(s.getState().agentChats.folders.map((f) => f.id)).toEqual(['f2'])
  })

  it('applyAgentChatFolders inserts a folder the store has never seen', () => {
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChatFolders([folder('f1', '', 0)])
    expect(s.getState().agentChats.folders).toEqual([folder('f1', '', 0)])
  })

  it('applyAgentChatFolders replaces an existing folder BY ID in place — a rename/renumber must not duplicate the row', () => {
    const s = createWorkspaceStore('w1')
    s.getState().seedAgentChatFolders([folder('f1', '', 0), folder('f2', '', 1)])
    s.getState().applyAgentChatFolders([folder('f2', '', 5, 'F2 renamed')])
    const folders = s.getState().agentChats.folders
    expect(folders).toHaveLength(2)
    expect(folders.find((f) => f.id === 'f2')).toMatchObject({ order: 5, name: 'F2 renamed' })
    expect(folders.find((f) => f.id === 'f1')).toMatchObject({ order: 0 }) // untouched sibling
  })

  it('applyAgentChatFolders upserts several rows in one call — the row a write named PLUS every sibling its dense renumber shifted', () => {
    const s = createWorkspaceStore('w1')
    s.getState().seedAgentChatFolders([folder('f1', '', 0)])
    s.getState().applyAgentChatFolders([folder('f1', '', 1), folder('f9', '', 0)]) // f9 is new, f1 renumbered
    const folders = s.getState().agentChats.folders
    expect(folders.find((f) => f.id === 'f1')?.order).toBe(1)
    expect(folders.find((f) => f.id === 'f9')?.order).toBe(0)
  })

  it('removeAgentChatFolder is a no-op for an id the store does not hold', () => {
    const s = createWorkspaceStore('w1')
    s.getState().seedAgentChatFolders([folder('f1', '', 0)])
    s.getState().removeAgentChatFolder('ghost')
    expect(s.getState().agentChats.folders.map((f) => f.id)).toEqual(['f1'])
  })

  it('removeAgentChatFolder deletes the folder and PROMOTES its chat children to its own parent — a folder holds no conversation, so the chat outlives it', () => {
    const s = createWorkspaceStore('w1')
    s.getState().seedAgentChatFolders([folder('f1', '', 0)])
    s.getState().applyAgentChat({
      ...chat('c1', '2026-01-01T00:00:00Z'),
      parentId: 'f1',
      order: 0,
    })

    s.getState().removeAgentChatFolder('f1')

    expect(s.getState().agentChats.folders).toEqual([])
    expect(s.getState().agentChats.chats.find((c) => c.id === 'c1')?.parentId).toBe('')
  })

  it('removeAgentChatFolder promotes nested FOLDER children too, not only chats', () => {
    const s = createWorkspaceStore('w1')
    s.getState().seedAgentChatFolders([folder('f1', '', 0), folder('f2', 'f1', 0)])

    s.getState().removeAgentChatFolder('f1')

    expect(s.getState().agentChats.folders.map((f) => f.id)).toEqual(['f2'])
    expect(s.getState().agentChats.folders[0].parentId).toBe('')
  })

  it('removeAgentChatFolder promotes children to the GRANDPARENT, not the tree root, when the deleted folder was itself nested', () => {
    const s = createWorkspaceStore('w1')
    s.getState().seedAgentChatFolders([
      folder('root-folder', '', 0),
      folder('f1', 'root-folder', 0),
    ])
    s.getState().applyAgentChat({
      ...chat('c1', '2026-01-01T00:00:00Z'),
      parentId: 'f1',
      order: 0,
    })

    s.getState().removeAgentChatFolder('f1')

    // f1's own parent was root-folder, not '' — its child must land THERE.
    expect(s.getState().agentChats.chats.find((c) => c.id === 'c1')?.parentId).toBe('root-folder')
  })

  it('setAgentChatPlacement moves a known chat, writing parentId and order', () => {
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().setAgentChatPlacement('c1', 'f1', 3)
    expect(s.getState().agentChats.chats[0]).toMatchObject({ parentId: 'f1', order: 3 })
  })

  it('setAgentChatPlacement is a no-op for an unknown chat id — nothing to move, nothing thrown', () => {
    const s = createWorkspaceStore('w1')
    expect(() => s.getState().setAgentChatPlacement('ghost', 'f1', 0)).not.toThrow()
    expect(s.getState().agentChats.chats).toEqual([])
  })

  // ── "Waiting in the terminal" ───────────────────────────────────────────
  // The modals no hook reports. PRESENCE in the map is the verdict, so an entry
  // with an EMPTY kind ("blocked, and we could not identify by what") must be
  // readable as blocked — the one thing a plain string map would get wrong.

  it('setAgentChatTerminalWait raises and clears the verdict', () => {
    const s = createWorkspaceStore('w1')

    setChatTerminalWait(s, 'c1', { kind: 'workspace_trust' })
    expect(s.getState().agentChats.terminalWaits.c1).toEqual({ kind: 'workspace_trust' })

    setChatTerminalWait(s, 'c1', null)
    expect(s.getState().agentChats.terminalWaits.c1).toBeUndefined()
  })

  it('an unidentified prompt is still an entry, with an empty kind', () => {
    const s = createWorkspaceStore('w1')

    setChatTerminalWait(s, 'c1', { kind: '' })

    expect(s.getState().agentChats.terminalWaits.c1?.kind).toBe('')
    expect('c1' in s.getState().agentChats.terminalWaits).toBe(true)
  })

  // An authoritative reconnect replaces the map from the server's own answer,
  // which is what repairs a terminal_wait frame dropped while the socket was down
  // — in BOTH directions.
  it('seedAgentChats replaces the wait map from the list response', () => {
    const s = createWorkspaceStore('w1')
    setChatTerminalWait(s, 'c1', { kind: 'workspace_trust' })

    s.getState().seedAgentChats([
      { ...chat('c1', '2026-01-01T00:00:00Z') },
      { ...chat('c2', '2026-01-02T00:00:00Z'), terminalWait: { kind: '' } },
    ])

    expect(s.getState().agentChats.terminalWaits).toEqual({ c2: { kind: '' } })
  })

  // A LIVE `created` reseed missed no frame, so it must leave standing answers
  // alone — clearing them here would take a banner down off a chat that is still
  // blocked, and nothing would put it back until the state next changed.
  it('removeAgentChat forgets the wait', () => {
    const s = createWorkspaceStore('w1')
    s.getState().applyAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    setChatTerminalWait(s, 'c1', { kind: '' })

    s.getState().removeAgentChat('c1')

    expect(s.getState().agentChats.terminalWaits.c1).toBeUndefined()
  })
})
