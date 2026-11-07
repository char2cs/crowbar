import { type AgentTerminalWait, TERMINAL_WAIT_TRUST } from '@/features/agent/api/agent-chat-reads'
import type { AskItem } from '@/features/agent/stores/lib/transcript-items'

/** What occupies the composer's slot: an input when you can talk, else the one
 *  question or reason you cannot. Never two stacked: a dead input under a live
 *  question invites an answer nobody reads. */
export type ComposerState =
  | { kind: 'input' }
  | { kind: 'choices'; asks: AskItem[] }
  | { kind: 'compacting' }
  | { kind: 'signpost'; reason: SignpostReason; message: string }

/** Why the bar is not an input, in the provider's terms rather than Crowbar's. */
export type SignpostReason =
  /** The chat is not known yet, so nothing can be sent to it. */
  | 'dormant'
  /** This provider declares no way to accept a typed prompt at all. */
  | 'unsupported'
  /** The CLI is blocked on a prompt that reaches the daemon through no hook. */
  | 'terminal_wait'
  /** The daemon is placing a CLI (a revive, a switch). */
  | 'reviving'
  /** The pane stopped following the chat's ledger, so an answer would never show. */
  | 'unfollowed'

/** The daemon placing a CLI this pane did not ask for — richer than the
 *  collapsed `live` boolean. A dormant chat is NOT one: sending revives it. */
export type ComposerRevival = { state: 'reviving'; message: string }

export interface ComposerInputs {
  live: boolean
  submitUnavailable: boolean
  terminalWait?: AgentTerminalWait
  compacting: boolean
  /** The asks still pending, oldest first. */
  asks: AskItem[]
  /** Why the chat's feed failed, while it has. */
  feedError?: string
}

/** What to say about a wait Crowbar cannot answer; an unknown kind is not guessed at. */
export function describeTerminalWait(wait: AgentTerminalWait): string {
  if (wait.kind === TERMINAL_WAIT_TRUST)
    return 'This provider is waiting for you to trust the workspace'
  return 'This provider is waiting for an answer only its terminal can give'
}

/** Resolves the bar in precedence order, the one thing a person can act on
 *  first: unfollowed, dormant, unsupported, terminal wait, ask, compacting, input.
 *  A failed turn is not here: its row says why, and the next prompt is the way on. */
export function resolveComposerState(inputs: ComposerInputs): ComposerState {
  const { live, submitUnavailable, terminalWait, compacting, asks } = inputs

  if (inputs.feedError) return { kind: 'signpost', reason: 'unfollowed', message: inputs.feedError }
  if (!live) return { kind: 'signpost', reason: 'dormant', message: 'Loading this chat…' }
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

  // An unanswerable ask still occupies the bar: the question is worth reading
  // even when only its terminal can answer; the card then draws no controls.
  if (asks.length > 0) return { kind: 'choices', asks }

  if (compacting) return { kind: 'compacting' }
  return { kind: 'input' }
}

/** Can the user type? Everything that is not an input queues or refuses. */
export function acceptsTyping(state: ComposerState): boolean {
  return state.kind === 'input' || state.kind === 'compacting'
}
