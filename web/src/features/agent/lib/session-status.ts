import type { ChatPhase, ChatSession } from '@/features/agent/api/agent-api'

/**
 * The pane's view of a chat's session, derived from the daemon's snapshot and
 * nothing else — the client never infers liveness or orchestrates a revive.
 *
 * `pending`: the chat list has not landed, so nothing is knowable yet.
 * `starting`: the daemon is placing a CLI (a revive, a switch, a replacement).
 * `live`: a runner is placed.
 * `dormant`: none is; sending a message revives it (server-side).
 */
export type SessionView =
  | { state: 'pending' }
  | { state: 'starting' }
  | { state: 'live' }
  | { state: 'dormant'; exitReason: string }

export interface SessionFacts {
  /** The chat list has landed and carries this chat. */
  known: boolean
  liveRunnerId: string
  phase: ChatPhase
  exitReason: string
}

export function sessionView({ known, liveRunnerId, phase, exitReason }: SessionFacts): SessionView {
  if (!known) return { state: 'pending' }
  if (liveRunnerId) return { state: 'live' }
  if (phase === 'starting' || phase === 'switching') return { state: 'starting' }
  return { state: 'dormant', exitReason }
}

/** Why a dormant chat has no CLI, in one sentence, and what continues it. */
export function describeDormant(exitReason: string): string {
  switch (exitReason) {
    case 'stopped':
      return 'Stopped. Send a message to continue the conversation.'
    case 'daemon_restart':
      return 'The agent ended when Crowbar restarted. Send a message to continue.'
    case 'connection_lost':
    case 'transport_overflow':
      return 'Crowbar lost its connection to the agent. Send a message to continue.'
    case 'resume_failed':
      return "The provider could not reopen its session. Your next message continues from Crowbar's transcript."
    case 'spawn_failed':
      return 'The agent could not start. Check that its CLI is installed, then try again.'
    default:
      return 'The agent is not running. Send a message to continue the conversation.'
  }
}

/** A note for a runner that could not resume the provider's own session and
 *  continued from Crowbar's transcript instead — undefined otherwise. */
export function describeRung(session: ChatSession | undefined): string | undefined {
  if (session?.rung !== 'transcript') return undefined
  return "Continued from Crowbar's transcript — the provider's own session was unavailable."
}
