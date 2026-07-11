import { useCallback, useEffect, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { Frame, FrameFooter, FramePanel } from '@/components/ui/frame'
import { getChat, resumeChat, switchProvider } from '@/features/agent/api/agent-api'
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'
import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { toast } from '@/features/window/stores/toast-store'
import { terminalListLive } from '@/lib/crowbar-bridge'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { ProviderSwitchDropdown } from './provider-switch-dropdown'

// attachAgentSegment resolves chatId's ACTIVE segment's live terminal session and
// pre-seeds the terminal-store mapping (connectionId = terminalSessionId) plus the
// localStorage reconnect backstop, so XtermTerminal's resolveTerminalConnection
// ATTACHES the agent's already-running PTY instead of spawning a fresh shell.
//
// Why this attaches (and does not spawn): XtermTerminal mounts with
// `sessionId = terminalSessionId`, reads getSession(sessionId).connectionId — now
// pre-seeded to the same id — and passes it to resolveTerminalConnection as
// `storeConnectionId`. On a fresh mount there is no live WS transport yet, so the
// resolver lists the daemon's live sessions, finds this PTY in them, and calls
// terminalAttach (the in-memory-store reuse branch).
//
// It returns null — do NOT mount a terminal — whenever the agent's PTY is not
// provably alive. That guard is load-bearing, because resolveTerminalConnection's
// fallback for an unknown connection id is createTerminal(): seeding a dead id
// would make it spawn a fresh BARE SHELL inside the agent pane and persist that
// shell into the reconnect map. The window is real — a CLI can exit before its
// EndSegment command lands, leaving the chat's active segment briefly pointing at
// a PTY the daemon has already reaped — so both facts are checked:
//
//  1. the active segment is still `active` (an `ended` segment's CLI has exited), and
//  2. the daemon still lists its terminal session as live.
//
// A failed liveness listing is treated as not-live for the same reason: never
// spawn. The caller renders an "agent session ended" state instead, and the WS
// stream's next segment_* frame re-runs this attach.
export async function attachAgentSegment(wsId: string, chatId: string): Promise<string | null> {
  const chat = await getChat(wsId, chatId)
  const segment = chat.segments.find((s) => s.id === chat.activeSegmentId)
  if (!segment || segment.status !== 'active') return null

  const sessionId = segment.terminalSessionId
  if (!sessionId) return null

  const live = await terminalListLive(`${workspaceBase(wsId)}/terminals`).catch(
    () => [] as string[],
  )
  if (!live.includes(sessionId)) return null

  useTerminalStore.getState().updateSession(sessionId, { connectionId: sessionId })
  saveReconnect(wsId, sessionId, sessionId)
  return sessionId
}

// The pane's attach outcome. `pending` is the pre-resolution state and renders
// nothing — distinguishing it from the others keeps an empty state from flashing
// on every open while the segment is being resolved. `reviving` is the dead-chat
// path: the CLI is gone and we are bringing it back into its own session.
//
// `idle` is a chat with no live CLI, which is NOT an end state — the ended segment
// still carries the provider and the native session id it bound, so the agent can
// always be brought back exactly where it left off. Its reason decides who does
// that:
//
//   'exited' — the CLI died under the OPEN pane (the user typed /exit, it
//              crashed, the daemon restarted). Reviving behind the user's back
//              here would resurrect a CLI they may have just deliberately quit,
//              so this offers a button instead.
//   'failed' — reviving was tried and could not start the CLI (not installed,
//              spawn failed). Retryable, and the footer's provider dropdown can
//              continue the conversation with a different provider.
type Attachment =
  | { state: 'pending' }
  | { state: 'attached'; sessionId: string }
  | { state: 'reviving' }
  | { state: 'idle'; reason: 'exited' | 'failed' }

// A revived CLI that dies on startup ends its segment at once, so an unbounded
// auto-revive would respawn it forever. Two attempts is enough to cover a
// transient failure while still settling fast on a permanent one.
const MAX_REVIVE_ATTEMPTS = 2

interface AgentChatPaneProps {
  chatId: string
  wsId: string
  /** The pane buffer backing this chat — the tab whose label tracks chat.title. */
  bufferId: string
  isActivePane: boolean
}

// Headerless CossUI Frame: a flush FramePanel holding the live agent terminal,
// and a FrameFooter whose only control is the provider-switch dropdown. The pane
// tab is keyed by the stable chatId; the inner terminal is keyed by the segment's
// terminalSessionId so a provider switch (new segment) remounts and re-attaches
// in place without disturbing the tab.
export function AgentChatPane({ chatId, wsId, bufferId, isActivePane }: AgentChatPaneProps) {
  const store = useWorkspaceStore()
  const activeSegmentId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.activeSegmentId ?? null,
  )
  const activeProviderId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.activeProviderId ?? '',
  )
  const title = useStore(store, (s) => s.agentChats.chats.find((c) => c.id === chatId)?.title ?? '')
  const providers = useStore(store, (s) => s.agentChats.providers)

  const [attachment, setAttachment] = useState<Attachment>({ state: 'pending' })

  // The tab label is a snapshot taken by openContent, but a chat's title changes
  // AFTER the tab opens: the agent auto-titles it (WS `title_set`) and the user
  // can rename it from the sidebar. Both land on this chat's `title` in the store,
  // so mirroring title → buffer name here keeps the open tab correct for both
  // paths, with no store → component dependency.
  useEffect(() => {
    if (!title) return
    const s = store.getState()
    const buffer = s.bufferActions.getBufferById(bufferId)
    if (buffer && buffer.name !== title) s.bufferActions.renameBuffer(bufferId, title)
  }, [store, bufferId, title])

  // Auto-revive is BOUNDED, not keyed on the segment. A revived CLI that dies on
  // startup ends its segment immediately, and each revive mints a NEW segment id —
  // so a guard keyed on "have I revived this segment?" would see a fresh key every
  // round and spawn CLIs forever. A counter cannot: after MAX_REVIVE_ATTEMPTS the
  // pane settles into idle and hands the user the button. Reset on a successful
  // attach (the chat is healthy again) and on an explicit Resume click.
  const reviveAttempts = useRef(0)

  // revive brings the chat's last provider back into its own native session.
  // Shared by the auto-revive on open and the explicit Resume button.
  const revive = useCallback(async () => {
    setAttachment({ state: 'reviving' })
    try {
      await resumeChat(wsId, chatId)
      // The revive spawned a CLI; its segment_opened frame lands on activeSegmentId
      // and re-runs the attach effect. Attach right away too, for the case where
      // the store already holds the new segment by the time this resolves.
      const sid = await attachAgentSegment(wsId, chatId)
      if (sid !== null) setAttachment({ state: 'attached', sessionId: sid })
    } catch (err: unknown) {
      console.error('Failed to resume agent chat:', err)
      setAttachment({ state: 'idle', reason: 'failed' })
      toast.error(
        'Could not resume this chat',
        'Crowbar could not restart the agent. Check that its CLI is installed and on your PATH.',
      )
    }
  }, [wsId, chatId])

  // Auto-revive belongs to OPENING a chat, and to nothing else. This effect also
  // re-runs whenever the active segment changes, and "no active segment" happens in
  // two situations that must be treated completely differently:
  //
  //   OPEN a chat whose CLI is gone  → revive it. The user asked for this chat back.
  //   The CLI DIES while you watch   → do NOT respawn it. You may have just typed
  //                                    /exit; bringing the agent straight back is
  //                                    the opposite of what you asked for, and it
  //                                    makes the agent impossible to quit (observed
  //                                    live: /exit, and claude reappeared instantly).
  //
  // The same distinction closes a race on every provider SWITCH: between the
  // segment_ended and segment_opened frames the chat momentarily has NO active
  // segment, and a revive fired in that gap would spawn a competing CLI alongside
  // the one the switch is already starting. Reviving only on the pane's FIRST
  // resolution can never fire there.
  const canAutoRevive = useRef(true)

  // Pointing the pane at a different chat is a fresh open, so it gets a fresh
  // budget: declared BEFORE the attach effect so it runs first on that change.
  useEffect(() => {
    canAutoRevive.current = true
    reviveAttempts.current = 0
  }, [wsId, chatId])

  useEffect(() => {
    let cancelled = false
    setAttachment({ state: 'pending' })

    void attachAgentSegment(wsId, chatId).then((sid) => {
      if (cancelled) return

      // Spend the "this is the open" budget on the FIRST resolution, whatever it
      // is. A chat that attached fine when opened and dies later is NOT an open —
      // that is the /exit case, and it must not respawn.
      const isOpen = canAutoRevive.current
      canAutoRevive.current = false

      if (sid !== null) {
        reviveAttempts.current = 0
        setAttachment({ state: 'attached', sessionId: sid })
        return
      }

      if (!isOpen || reviveAttempts.current >= MAX_REVIVE_ATTEMPTS) {
        setAttachment({
          state: 'idle',
          reason: reviveAttempts.current > 0 ? 'failed' : 'exited',
        })
        return
      }
      reviveAttempts.current += 1
      void revive()
    })

    return () => {
      cancelled = true
    }
  }, [wsId, chatId, activeSegmentId, revive])

  // The mount guard (attachAgentSegment) only proves the PTY was alive at OPEN.
  // The agent's CLI can die at any moment while the pane sits here — daemon
  // restart, CLI exit, crash — and the terminal's transport-drop reconnect would,
  // by default, resolve the now-dead session by spawning a fresh BARE SHELL into
  // this frame. So the terminal runs attach-only and reports the session gone
  // instead; we render the same ended state the mount guard does. The backend's
  // boot reconcile independently ends the segment and the WS `segment_ended` frame
  // re-runs the effect above — but this guard must stand on its own, because the
  // PTY's death and the daemon's knowledge of it are not the same instant.
  // Deliberately NOT an auto-revive: the CLI just died in front of the user, who
  // may have quit it themselves (/exit). Respawning it behind their back would be
  // a surprise. They get a Resume button instead — and simply reopening the chat
  // later revives it, because that reopen is an explicit "I want this chat back".
  const handleSessionGone = useCallback(
    () => setAttachment({ state: 'idle', reason: 'exited' }),
    [],
  )

  // A provider switch is the headline interaction of this pane, and it can fail
  // for real, ordinary reasons — the target CLI is not installed, the spawn fails.
  // Without this catch the rejection is unhandled: the dropdown just closes and
  // nothing happens. Surface it, matching the write-path error handling in
  // agent-chats-panel (create/rename/delete).
  const handleSwitch = (providerId: string) => {
    switchProvider(wsId, chatId, providerId).catch((err: unknown) => {
      console.error('Failed to switch agent provider:', err)
      const name = providers.find((p) => p.id === providerId)?.displayName ?? providerId
      toast.error(
        'Could not switch provider',
        `Crowbar could not start ${name}. Check that its CLI is installed and on your PATH.`,
      )
    })
  }

  return (
    <Frame className="h-full w-full rounded-none bg-transparent p-0">
      <FramePanel className="min-h-0 flex-1 rounded-none border-0 bg-transparent p-0 shadow-none before:hidden">
        {attachment.state === 'attached' && (
          <XtermTerminal
            key={attachment.sessionId}
            sessionId={attachment.sessionId}
            isActive={isActivePane}
            isVisible
            attachOnly
            onSessionGone={handleSessionGone}
          />
        )}
        {attachment.state === 'reviving' && (
          <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-6">
            <FlickerSpinner className="text-muted-foreground size-6" />
            <p className="text-muted-foreground text-center text-sm">Resuming this chat…</p>
          </div>
        )}
        {attachment.state === 'idle' && (
          <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-6">
            <p className="text-muted-foreground max-w-sm text-center text-sm">
              {attachment.reason === 'failed'
                ? 'Crowbar could not restart this agent. Check that its CLI is installed, then try again — or pick another provider below.'
                : 'This agent has exited. Resume it to pick the conversation up where you left off.'}
            </p>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => {
                reviveAttempts.current = 0
                void revive()
              }}
            >
              Resume
            </Button>
          </div>
        )}
      </FramePanel>
      <FrameFooter className="flex items-center justify-start px-2 py-1.5">
        <ProviderSwitchDropdown
          providers={providers}
          currentProviderId={activeProviderId}
          onSwitch={handleSwitch}
        />
      </FrameFooter>
    </Frame>
  )
}
