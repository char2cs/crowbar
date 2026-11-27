import { type AgentTerminalWait, TERMINAL_WAIT_TRUST } from '@/features/agent/api/agent-chat-reads'
import type { AskItem } from '@/features/agent/stores/lib/transcript-items'

export type ComposerState =
  | { kind: 'input' }
  | { kind: 'choices'; asks: AskItem[] }
  | { kind: 'compacting' }
  | { kind: 'signpost'; reason: SignpostReason; message: string }

export type SignpostReason =
  'dormant' | 'unsupported' | 'terminal_wait' | 'reviving' | 'unfollowed' | 'offline'

export type ComposerRevival = { state: 'reviving'; message: string }

export interface ComposerInputs {
  live: boolean
  offline?: boolean
  submitUnavailable: boolean
  terminalWait?: AgentTerminalWait
  compacting: boolean
  asks: AskItem[]
  feedError?: string
}

function describeTerminalWait(wait: AgentTerminalWait): string {
  if (wait.kind === TERMINAL_WAIT_TRUST)
    return 'This provider is waiting for you to trust the workspace'
  return 'This provider is waiting for an answer only its terminal can give'
}

export function resolveComposerState(inputs: ComposerInputs): ComposerState {
  const { live, submitUnavailable, terminalWait, compacting, asks } = inputs

  if (inputs.offline) return { kind: 'signpost', reason: 'offline', message: 'Offline — read-only' }
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

  if (asks.length > 0) return { kind: 'choices', asks }

  if (compacting) return { kind: 'compacting' }
  return { kind: 'input' }
}

export function acceptsTyping(state: ComposerState): boolean {
  return state.kind === 'input' || state.kind === 'compacting'
}
