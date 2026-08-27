import type { AgentActivity, AgentChoice, AgentTerminalWait } from '@/features/agent/api/agent-api'
import { blockedOn, pendingChoices } from '@/features/agent/lib/agent-activity'

/**
 * What occupies the composer's slot.
 *
 * THE RULE: it is an input when you can talk, and it is the question, the
 * permission, or the reason you cannot, when you cannot. One box, one occupant,
 * never two stacked — a dead input rendered beneath a live question invites
 * someone to type an answer into a field nobody reads.
 */
export type ComposerState =
  | { kind: 'input' }
  | { kind: 'choice'; choice: AgentChoice }
  | { kind: 'halted'; message: string; resetsAt?: string }
  | { kind: 'compacting' }
  | { kind: 'signpost'; reason: SignpostReason; message: string }

/** Why the bar is not an input, in the provider's terms rather than Crowbar's. */
export type SignpostReason =
  /** No runner. There is nothing on the other end of the box. */
  | 'dormant'
  /** This provider declares no way to accept a typed prompt at all. */
  | 'unsupported'
  /** The CLI is blocked on a prompt that reaches the daemon through no hook. */
  | 'terminal_wait'
  /** Crowbar's own revive is in flight, unasked. */
  | 'reviving'
  /** A revive was tried (automatically, or by hand) and there is nothing left on
   *  the other end of the box until a person tries again. */
  | 'idle'

/** The pane's own revive attempt for a chat that is not live — richer than the
 *  collapsed `live` boolean, which cannot tell "still resolving" from "actively
 *  resuming" from "gave up; needs a person". Undefined for a chat whose
 *  liveness has not even been read back yet. */
export type ComposerRevival =
  { state: 'reviving'; message: string } | { state: 'idle'; reason: 'exited' | 'failed' }

export interface ComposerInputs {
  live: boolean
  revival?: ComposerRevival
  submitUnavailable: boolean
  terminalWait?: AgentTerminalWait
  compacting: boolean
  activity: AgentActivity
  /** The provider's own words about why it stopped, from a `notice` message. */
  haltedMessage?: string
  haltedResetsAt?: string
}

export const TERMINAL_WAIT_TRUST = 'workspace_trust'

/** What to say about a wait Crowbar cannot answer. A kind this build has never
 *  heard of says exactly that and does not guess — a newer daemon minting a new
 *  kind must never make an older client say something wrong. */
export function describeTerminalWait(wait: AgentTerminalWait): string {
  if (wait.kind === TERMINAL_WAIT_TRUST)
    return 'This provider is waiting for you to trust the workspace'
  return 'This provider is waiting for an answer only its terminal can give'
}

/**
 * Resolve the bar, in strict precedence order.
 *
 * The order is the contract, and it reads top-down as "what is the ONE thing a
 * person can act on right now":
 *
 *  1. `dormant`      — no runner exists; nothing else is even reachable
 *                      (refined into `reviving`/`idle` by `revival`, when the
 *                      pane can say more than just "not live")
 *  2. `unsupported`  — this provider will never take a typed prompt
 *  3. `terminal_wait`— the CLI is blocked where only its terminal can reach
 *  4. `unanswerable` — a question whose answer cannot be delivered from here
 *  5. `choice`       — a question with buttons that work
 *  6. `halted`       — the provider stopped this turn and said why
 *  7. `compacting`   — busy; prompts queue
 *  8. `input`
 *
 * A chat blocked on a trust dialog AND holding a pending choice shows the trust
 * dialog, because that is the one a person can actually do something about.
 */
export function resolveComposerState(inputs: ComposerInputs): ComposerState {
  const { live, revival, submitUnavailable, terminalWait, compacting, activity } = inputs

  if (!live) {
    if (revival?.state === 'reviving') {
      return { kind: 'signpost', reason: 'reviving', message: revival.message }
    }
    if (revival?.state === 'idle') {
      return {
        kind: 'signpost',
        reason: 'idle',
        message:
          revival.reason === 'failed'
            ? 'Crowbar could not restart this agent. Check that its CLI is installed, then try again — or pick another provider below.'
            : 'This agent has exited. Resume it to pick the conversation up where you left off.',
      }
    }
    return {
      kind: 'signpost',
      reason: 'dormant',
      message: 'Resume the provider before sending from Chat.',
    }
  }
  if (submitUnavailable) {
    return {
      kind: 'signpost',
      reason: 'unsupported',
      message: 'This provider cannot accept a prompt typed here.',
    }
  }
  if (terminalWait) {
    return {
      kind: 'signpost',
      reason: 'terminal_wait',
      message: describeTerminalWait(terminalWait),
    }
  }

  // The OLDEST pending prompt, answerable or not.
  //
  // An unanswerable one still occupies the bar rather than becoming a bare
  // signpost, because the question itself is the thing worth reading: a provider
  // can report a permission it has no way to receive a reply for — codex is the
  // shipped case, whose descriptor says so outright — and hiding that behind
  // "answer in the terminal" is what made a blocked agent look frozen. The
  // component draws the question either way and the CONTROLS only when they can
  // reach someone.
  const first = pendingChoices(activity)[0]
  if (first) return { kind: 'choice', choice: first }

  if (inputs.haltedMessage) {
    return {
      kind: 'halted',
      message: inputs.haltedMessage,
      resetsAt: inputs.haltedResetsAt,
    }
  }
  if (compacting) return { kind: 'compacting' }
  return { kind: 'input' }
}

/** Can the user type? Everything that is not an input queues or refuses. */
export function acceptsTyping(state: ComposerState): boolean {
  return state.kind === 'input' || state.kind === 'compacting'
}

/** The interruption worth naming beside the bar, if any. Only the latest — they
 *  are states, not a log, and a stack of them tells a reader nothing the top one
 *  does not. */
export function currentInterruption(activity: AgentActivity) {
  return blockedOn(activity)
}
