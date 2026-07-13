import { useCallback, useEffect, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { getChat, resumeChat, switchProvider } from '@/features/agent/api/agent-api'
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'
import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { toast } from '@/features/window/stores/toast-store'
import { ProviderSwitchDropdown } from './provider-switch-dropdown'

// seedAttach pre-seeds the terminal-store mapping (connectionId = terminalSessionId)
// plus the localStorage reconnect backstop, so XtermTerminal's
// resolveTerminalConnection ATTACHES the agent's already-running PTY instead of
// spawning a fresh shell.
//
// Why this attaches (and does not spawn): XtermTerminal mounts with
// `sessionId = terminalSessionId`, reads getSession(sessionId).connectionId — now
// pre-seeded to the same id — and passes it to resolveTerminalConnection as
// `storeConnectionId`. On a fresh mount there is no live WS transport yet, so the
// resolver lists the daemon's live sessions, finds this PTY among them, and calls
// terminalAttach (the in-memory-store reuse branch).
//
// It must therefore NEVER be handed a dead PTY: resolveTerminalConnection's fallback
// for an unknown connection id is createTerminal(), so seeding a dead id would spawn a
// BARE SHELL inside the agent pane and persist that shell into the reconnect map.
//
// The caller's guard used to be TWO questions — is the segment still `active`, AND does
// the daemon still list its PTY as live — because those two could disagree. They cannot
// any more. A chat's terminalSessionId comes from its LIVE RUNNER, and a live-runner row
// exists exactly while its PTY does, so `liveRunnerId` being present IS the liveness
// answer. One authority, no second round trip, nothing left to drift.
function seedAttach(wsId: string, terminalSessionId: string): void {
  useTerminalStore.getState().updateSession(terminalSessionId, { connectionId: terminalSessionId })
  saveReconnect(wsId, terminalSessionId, terminalSessionId)
}

// The pane's attach outcome.
//
// `pending` is the pre-resolution state and renders nothing. It is NOT `idle`: it means
// the chat list has not reached the store yet, so we do not know whether this chat is
// live. Flashing the Resume button here would offer to spawn a second CLI onto a chat
// that may well already have one.
//
// `idle` is a chat with NO RUNNER on it — dormant. Not an end state: the chat's last
// conversation still carries the provider and the native session id it bound, so the
// agent can always be brought back exactly where it left off. Its reason decides what
// we say:
//
//   'exited' — no CLI is on this chat. It quit (/exit), crashed, or died with the
//              daemon. The user gets a button; nothing is respawned behind their back,
//              because they may have just deliberately quit it.
//   'failed' — a Resume was tried and could not start the CLI (not installed, spawn
//              failed). Retryable, and the footer's dropdown can continue the
//              conversation with a different provider instead.
type Attachment =
  | { state: 'pending' }
  | { state: 'attached'; sessionId: string }
  | { state: 'reviving' }
  | { state: 'idle'; reason: 'exited' | 'failed' }

interface AgentChatPaneProps {
  /** The chat this tab is pointed at. NOT stable for its life: the pane re-points it. */
  chatId: string
  /** The runner this tab follows, or '' when it has none (a dormant chat). */
  runnerId: string
  wsId: string
  /** The pane buffer backing this tab — the thing the pane re-points and relabels. */
  bufferId: string
  isActivePane: boolean
}

// A flat, opaque pane: one centred column holding the live agent terminal, with the
// provider-switch dropdown beneath it on the same column. See the render for why this
// is NOT a card.
//
// THE TAB IS A VIEWPORT ON A MOVING TARGET. What a chat pane shows is not a chat, it is
// a RUNNER — the vendor-CLI process — and that process moves:
//
//   the runner MOVES to another chat (the user types /clear or /resume INSIDE the CLI,
//     and it switches conversation). The pane follows it: chatId is re-pointed, the tab
//     relabels — and because the terminal is keyed by the runner's PTY, which a move
//     does not change, XTERM NEVER REMOUNTS. The conversation changes without the
//     terminal changing. Pinning the pane to a chatId is what produced the bug this
//     replaces: the tab said "this agent has exited" and offered a Resume button that
//     spawned a SECOND CLI, while the first was alive and well in a chat the user then
//     had to go and find.
//
//   the runner is REPLACED on the same chat (a provider switch, or a Resume of a dormant
//     chat). The pane adopts the runner now on its chat: runnerId is re-pointed, and
//     since that runner has a PTY of its own, the terminal re-attaches.
//
// Both are one rule, applied in order: FOLLOW MY RUNNER IF IT STILL EXISTS ANYWHERE;
// OTHERWISE ADOPT WHOEVER IS ON MY CHAT. Attaching, relabelling and the exited state all
// fall out of it rather than being engineered separately.
export function AgentChatPane({
  chatId,
  runnerId,
  wsId,
  bufferId,
  isActivePane,
}: AgentChatPaneProps) {
  const store = useWorkspaceStore()

  // Where is MY runner? '' when it is nowhere — it exited, or a switch replaced it. A
  // chat is live exactly while a runner is placed on it, so this lookup is also what
  // makes a MOVE visible: the runner turns up under a different chat id.
  const runnerChatId = useStore(store, (s) =>
    runnerId ? (s.agentChats.chats.find((c) => c.liveRunnerId === runnerId)?.id ?? '') : '',
  )

  // The chat this pane is SHOWING: my runner's chat if it still has one (it moved, and
  // the tab follows), else the chat the tab was pointed at (which may be dormant).
  const shownChatId = runnerChatId || chatId

  // Does the store KNOW this chat at all? "Not in the store yet" (the seed is in flight)
  // is not "dormant", and must not render the Resume button — see `pending` above.
  const known = useStore(store, (s) => s.agentChats.chats.some((c) => c.id === shownChatId))
  // The runner on the shown chat — mine, or whoever replaced it. '' = dormant.
  const liveRunnerId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.liveRunnerId ?? '',
  )
  // That runner's PTY. Empty exactly when liveRunnerId is: no runner, nothing to attach.
  const sessionId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.terminalSessionId ?? '',
  )
  const activeProviderId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.activeProviderId ?? '',
  )
  const title = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === shownChatId)?.title ?? '',
  )
  const providers = useStore(store, (s) => s.agentChats.providers)

  const [attachment, setAttachment] = useState<Attachment>({ state: 'pending' })

  // The two layout divs whose empty space belongs to the terminal, and the terminal's
  // own imperative handle — see focusTerminalFromEmptySpace.
  const rootRef = useRef<HTMLDivElement>(null)
  const columnRef = useRef<HTMLDivElement>(null)
  const terminalApiRef = useRef<{ focus: () => void } | null>(null)

  // A TERMINAL DOES NOT RENDER TO THE EDGE OF ITS OWN BOX.
  //
  // xterm draws a character grid: it fits `floor(width / cellWidth)` columns and also
  // holds back room for a scrollbar, so the canvas is NARROWER than the element that
  // contains it — here by 16px. That strip is never drawn to. Align the status line to
  // the terminal's ELEMENT and it lands perfectly on empty space, sitting visibly past
  // where the agent's text actually stops. (Which is exactly what happened: the box was
  // flush to the pixel while the switcher hung ~25px beyond the last column.)
  //
  // So measure the real thing — the canvas — and pad the status line by the difference.
  // It cannot be a constant: it moves with the font metrics, the pane width, and the
  // scrollbar reservation.
  const [gridSlack, setGridSlack] = useState(0)

  useEffect(() => {
    const column = columnRef.current
    if (attachment.state !== 'attached' || !column) {
      setGridSlack(0)
      return
    }

    const measure = () => {
      const element = column.querySelector('.xterm')
      const canvas = column.querySelector('.xterm canvas')
      // No canvas yet — the renderer has not drawn. The MutationObserver below will call
      // us back the moment it appears; do NOT fall back to 0, that is what pinned the
      // status line to the element's edge instead of the grid's.
      if (!element || !canvas) return
      const slack = element.getBoundingClientRect().right - canvas.getBoundingClientRect().right
      setGridSlack(Math.max(0, Math.round(slack)))
    }

    // Two different events change the answer, and BOTH are needed.
    //
    // MutationObserver — xterm builds its canvas asynchronously, after this effect's
    // first frame. Measuring only on mount reads a terminal that has not rendered yet
    // and silently yields 0, and since nothing then resizes, it stays 0 forever. That is
    // precisely the bug that made this look "already aligned" while it wasn't.
    //
    // ResizeObserver — on relayout xterm re-fits to a new column count, so the slack
    // changes. Measured a frame later, because the observer fires when the CONTAINER
    // resizes and xterm re-fits after that; same-tick reads the stale canvas.
    let frame = 0
    const remeasure = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(measure)
    }

    const resize = new ResizeObserver(remeasure)
    resize.observe(column)

    const mutation = new MutationObserver(remeasure)
    mutation.observe(column, { childList: true, subtree: true })

    remeasure()

    return () => {
      cancelAnimationFrame(frame)
      resize.disconnect()
      mutation.disconnect()
    }
  }, [attachment.state])

  // Re-point the buffer at what this pane is actually showing. This is the write that
  // makes the tab follow: pane-container feeds the buffer's chatId/runnerId straight
  // back in as our props, so the next render is already looking at the new chat.
  //
  // It converges in one step and cannot loop — once the buffer holds the pair computed
  // here, the effect's deps are unchanged and it does not re-run. Writing '' as the
  // runnerId of a dormant chat is deliberate: a tab must not go on claiming a runner
  // that no longer exists.
  useEffect(() => {
    if (!known) return // nothing authoritative to re-point at yet
    if (shownChatId === chatId && liveRunnerId === runnerId) return
    store.getState().bufferActions.repointAgentChatBuffer(bufferId, {
      chatId: shownChatId,
      runnerId: liveRunnerId,
    })
  }, [store, bufferId, known, chatId, runnerId, shownChatId, liveRunnerId])

  // The tab label is a snapshot taken by openContent, but the title changes AFTER the
  // tab opens: the agent auto-titles the chat (WS `title_set`), the user renames it from
  // the sidebar — and the tab may now be showing a DIFFERENT chat than it opened on,
  // which has a title of its own. All three land on the shown chat's `title`, so
  // mirroring title → buffer name here keeps the tab honest for all of them, with no
  // store → component dependency.
  useEffect(() => {
    if (!title) return
    const s = store.getState()
    const buffer = s.bufferActions.getBufferById(bufferId)
    if (buffer && buffer.name !== title) s.bufferActions.renameBuffer(bufferId, title)
  }, [store, bufferId, title])

  // Attach to the runner's PTY, or settle into the dormant state. The seeding must
  // happen BEFORE XtermTerminal mounts (React runs child effects first, so a terminal
  // rendered in the same commit would resolve its connection against an unseeded store)
  // — hence the state machine: this effect seeds, flips to `attached`, and the terminal
  // mounts on the next render.
  //
  // There is no auto-revive, and no retry budget. A pane that could not tell "my CLI
  // died" from "my CLI moved" needed both — it had to guess, and a bounded counter was
  // the only thing stopping it spawn-looping. This one can tell, so reviving is
  // something the USER does, explicitly, with the button. Nothing implicit.
  useEffect(() => {
    if (!known) {
      setAttachment({ state: 'pending' })
      return
    }
    if (!sessionId) {
      // Don't stomp the spinner of a Resume still in flight; it sets its own terminal
      // state when it lands.
      setAttachment((a) => (a.state === 'reviving' ? a : { state: 'idle', reason: 'exited' }))
      return
    }
    seedAttach(wsId, sessionId)
    setAttachment({ state: 'attached', sessionId })
  }, [wsId, known, sessionId])

  // revive brings the chat's last provider back into its own native session. Only ever
  // called from the Resume button.
  //
  // The refetch is not belt-and-braces: resume's WS frames would update the store
  // eventually, but reading the chat back here means the pane attaches on the CLICK
  // rather than whenever a frame happens to arrive — and it settles honestly if the
  // revived CLI died on startup (resumed, but nothing on the chat).
  const revive = useCallback(async () => {
    setAttachment({ state: 'reviving' })
    try {
      await resumeChat(wsId, shownChatId)
      const chat = await getChat(wsId, shownChatId)
      const s = store.getState()
      s.upsertAgentChat(chat)
      if (!chat.liveRunnerId || !chat.terminalSessionId) {
        setAttachment({ state: 'idle', reason: 'failed' })
        return
      }
      s.bufferActions.repointAgentChatBuffer(bufferId, {
        chatId: chat.id,
        runnerId: chat.liveRunnerId,
      })
      seedAttach(wsId, chat.terminalSessionId)
      setAttachment({ state: 'attached', sessionId: chat.terminalSessionId })
    } catch (err: unknown) {
      console.error('Failed to resume agent chat:', err)
      setAttachment({ state: 'idle', reason: 'failed' })
      toast.error(
        'Could not resume this chat',
        'Crowbar could not restart the agent. Check that its CLI is installed and on your PATH.',
      )
    }
  }, [store, wsId, bufferId, shownChatId])

  // The attach above proves the PTY was alive when the store last spoke. The CLI can die
  // at any moment while the pane sits here — daemon restart, /exit, crash — and the
  // terminal's transport-drop reconnect would, by default, resolve the now-dead session
  // by spawning a fresh BARE SHELL into this frame. So the terminal runs attach-only and
  // reports the session gone instead, and we render the dormant state at once rather
  // than waiting for the backend to notice: the PTY's death and the daemon's knowledge
  // of it are not the same instant.
  //
  // A MOVE never lands here — a runner keeps its PTY when it changes conversation, so the
  // terminal sees nothing at all. That is exactly why this can now be read as "the CLI
  // died" with no ambiguity, and why the pane no longer needs to guess.
  const handleSessionGone = useCallback(
    () => setAttachment({ state: 'idle', reason: 'exited' }),
    [],
  )

  // Switch the provider ON THE CHAT THE RUNNER IS IN NOW (shownChatId — not the chatId
  // the tab was opened on): after a /clear the pane is showing a different conversation,
  // and switching the one the user has left would spawn a CLI onto abandoned context
  // while the live one kept running unattended.
  //
  // A switch can fail for real, ordinary reasons — the target CLI is not installed, the
  // spawn fails. Without this catch the rejection is unhandled: the dropdown just closes
  // and nothing happens. Surface it, matching the write-path error handling in
  // agent-chats-panel (create/rename/delete).
  const handleSwitch = (providerId: string) => {
    switchProvider(wsId, shownChatId, providerId).catch((err: unknown) => {
      console.error('Failed to switch agent provider:', err)
      const name = providers.find((p) => p.id === providerId)?.displayName ?? providerId
      toast.error(
        'Could not switch provider',
        `Crowbar could not start ${name}. Check that its CLI is installed and on your PATH.`,
      )
    })
  }

  // Clicking the gutters or the column's padding focuses the terminal.
  //
  // Those regions LOOK like part of the chat — they are the same bg-background, and the
  // whole point of the centred column is that its surroundings read as breathing room
  // rather than as somewhere else. But they are plain divs, so by default a mousedown
  // there does the opposite of what it looks like: it BLURS the terminal's textarea and
  // the user's next keystroke goes nowhere. preventDefault is what stops that blur; the
  // focus() then puts the caret back where the user obviously meant to click.
  //
  // The guard is a WHITELIST — the target must BE the root or the column, not merely be
  // inside them. A blacklist ("not the terminal, not a button") would silently start
  // hijacking clicks the day anything else is added to this pane: the switcher, its
  // menu, a future toolbar. Landing directly on a layout div is exactly what "the user
  // clicked empty space" means, and nothing else can accidentally match it.
  const focusTerminalFromEmptySpace = (e: React.MouseEvent) => {
    if (attachment.state !== 'attached') return
    if (e.target !== rootRef.current && e.target !== columnRef.current) return
    e.preventDefault()
    terminalApiRef.current?.focus()
  }

  return (
    // One flat surface — no card, no border, no raised panel, and NO background of its
    // own. The pane's content region already paints `bg-pane-background`, which is the
    // surface every pane in Crowbar sits on; an editor pane shows it by staying
    // transparent, and so do we. Painting an opaque bg-background here did not bleed
    // onto anything — it COVERED that shared surface, which is exactly why the chat
    // read as a different material from the Monaco tab next to it.
    //
    // We built this on CossUI's Frame first, faithfully, and seeing it live is what
    // settled it: a Frame's whole job is to lift a panel OFF its background, and a
    // chat pane does not want to be lifted off anything. The bordered card framed
    // the agent's empty middle instead of hiding it, its squared top had nothing to
    // meet once the column was centred, and the switcher — outside the card — read
    // as a stray button on the desktop. Frame itself is untouched and still there
    // for surfaces that DO want to be raised.
    //
    // The gutters and the column's padding are DEAD SPACE that looks like part of the
    // chat, so clicking them focuses the terminal instead of blurring it — see
    // focusTerminalFromEmptySpace.
    <div
      ref={rootRef}
      onMouseDown={focusTerminalFromEmptySpace}
      className="flex h-full w-full flex-col"
    >
      {/* THE COLUMN. Both the terminal and the switcher live inside it, and the
          padding is on the column rather than on either of them. That is the whole
          trick: the switcher cannot drift out of line with the agent's first
          character, because they are inset by the SAME box. Alignment is structural
          here, not a hand-tuned pixel — which is exactly what it was before, and it
          broke every time anything moved.

          The cap is what makes the padding appear only when there is room to spare:
          wide pane → real gutters; narrow pane → the column just fills it. And it
          resizes the PTY, so the agent genuinely re-wraps to ~106 columns instead of
          running lines to 164. Because the whole pane is one bg-background, the
          padding and the gutters are invisible — you see breathing room, not a box. */}
      <div
        ref={columnRef}
        className="mx-auto flex min-h-0 w-full max-w-4xl flex-1 flex-col px-4 pt-4"
      >
        <div className="min-h-0 flex-1">
          {attachment.state === 'attached' && (
            <XtermTerminal
              key={attachment.sessionId}
              sessionId={attachment.sessionId}
              isActive={isActivePane}
              isVisible
              attachOnly
              // The column already supplies the inset; the terminal's own pl-[16px]
              // would double it and push the agent out of line with the switcher.
              flush
              onTerminalRef={(api) => {
                terminalApiRef.current = api
              }}
              onSessionGone={handleSessionGone}
            />
          )}
          {attachment.state === 'reviving' && (
            <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-6">
              <FlickerSpinner className="size-6 text-foreground" />
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
              <Button type="button" variant="secondary" size="sm" onClick={() => void revive()}>
                Resume
              </Button>
            </div>
          )}
        </div>

        {/* The chat's own status line, spanning the terminal's width: what this
            conversation IS on the left, who is running it on the right. Both sit on the
            column, so the title starts on the agent's first character and the switcher
            ends on its last one.
            min-w-0 is what lets a long title truncate instead of shoving the switcher
            off the column — a flex child defaults to min-width:auto and refuses to
            shrink below its content. */}
        <div
          className="flex items-center justify-between gap-3 py-2"
          style={{ paddingRight: gridSlack }}
        >
          <span className="min-w-0 truncate text-muted-foreground text-sm">{title}</span>
          <ProviderSwitchDropdown
            providers={providers}
            currentProviderId={activeProviderId}
            onSwitch={handleSwitch}
          />
        </div>
      </div>
    </div>
  )
}
