import { TerminalIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { TERMINAL_WAIT_TRUST } from '@/features/agent/api/agent-api'

interface AgentTerminalWaitBannerProps {
  /** Crowbar's own name for the prompt, or '' when it knows only that one is up. */
  kind: string
  /** The provider's display name, or '' when it is not known yet. */
  providerLabel: string
  onOpenTerminal: () => void
}

/** What the chat says when its CLI is blocked on something Crowbar CANNOT answer.
 *
 *  This is the counterpart of the choice card, and the difference between them is
 *  the whole point. A choice arrived over a hook, so the chat shows it with
 *  buttons and answering it there works. This one did not — a workspace-trust
 *  dialog, a first-run screen, a login — and there is no channel to reply on at
 *  all. So the banner does not offer to answer; it says what is happening and
 *  points at the only place it can be dealt with.
 *
 *  It exists because the alternative was NOTHING: no spinner, no message, no
 *  error, a pane that looked dead over a live, blocked process, which is the
 *  single most common way Crowbar has looked broken when it was not.
 *
 *  It NEVER guesses. A kind Crowbar recognises gets a sentence naming the prompt;
 *  anything else — including a kind minted by a newer daemon than this client —
 *  falls back to saying only that input is wanted, which is all that is actually
 *  known.
 *
 *  STAYS pane-level rather than moving into the composer alongside
 *  dormant/unsupported: the composer is unreachable for a chat with no messages
 *  yet, and a workspace-trust prompt is disproportionately a FIRST-TURN event —
 *  see the note where this renders in agent-chat-pane.tsx. Rendered in NORMAL
 *  FLOW there, not as an `absolute` overlay: a fixed offset under an absolute
 *  box reserves no space for its own height, so whatever sat beneath it never
 *  moved when this wrapped to more lines — measured as a real, worsening
 *  overlap with the composer as the pane narrowed. */
export function AgentTerminalWaitBanner({
  kind,
  providerLabel,
  onOpenTerminal,
}: AgentTerminalWaitBannerProps) {
  const who = providerLabel || 'The agent'
  const what =
    kind === TERMINAL_WAIT_TRUST
      ? `${who} is asking whether you trust this folder.`
      : `${who} is waiting for input in its terminal.`

  return (
    // The same card the choice prompt and the interruption strip wear, because it
    // is the same class of thing to the reader: the agent has stopped, and it
    // wants something from you.
    <div
      className="flex items-center justify-between gap-3 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-sm"
      role="alert"
      data-testid="agent-terminal-wait"
      data-wait-kind={kind}
    >
      <p className="min-w-0 text-warning-foreground">
        {what} <span className="text-muted-foreground">Crowbar can’t answer this one.</span>
      </p>
      <Button className="shrink-0" size="xs" variant="secondary" onClick={onOpenTerminal}>
        <TerminalIcon /> Open Terminal
      </Button>
    </div>
  )
}

interface AgentReturnToChatNoticeProps {
  onReturn: () => void
  onDismiss: () => void
}

/** The way back, for a user Crowbar moved to the terminal and cannot move back.
 *
 *  Crowbar returns people to the chat automatically when it can (see the pane's
 *  escort state). It deliberately cannot when they have since navigated
 *  elsewhere, or chosen a surface themselves — yanking someone out of a terminal
 *  they are using is worse than the problem it solves.
 *
 *  But leaving them there silently is worse still: they were sent here by
 *  Crowbar, for a reason that has now gone away, and nothing on screen would ever
 *  say so. So it offers, and it takes no for an answer. */
export function AgentReturnToChatNotice({ onReturn, onDismiss }: AgentReturnToChatNoticeProps) {
  return (
    <div
      className="flex items-center justify-between gap-3 border-t py-2 text-muted-foreground text-xs"
      role="status"
      data-testid="agent-return-to-chat"
    >
      <span>The agent is no longer waiting on you here.</span>
      <div className="flex items-center gap-1">
        <Button size="xs" variant="ghost" onClick={onReturn}>
          Return to Chat
        </Button>
        <Button size="xs" variant="ghost" onClick={onDismiss}>
          Dismiss
        </Button>
      </div>
    </div>
  )
}
