import type { KeyboardEvent } from 'react'
import type {
  AgentActivity,
  AgentTerminalWait,
  PermissionLevel,
} from '@/features/agent/api/agent-api'
import { ComposerChoice } from '@/features/agent/composer/composer-choice'
import { ComposerField } from '@/features/agent/composer/composer-field'
import { ComposerHalted } from '@/features/agent/composer/composer-halted'
import { ComposerHandle } from '@/features/agent/composer/composer-handle'
import { ComposerSignpost } from '@/features/agent/composer/composer-signpost'
import {
  resolveComposerState,
  type ComposerRevival,
} from '@/features/agent/composer/lib/composer-state'
import { isMultiline } from '@/features/agent/composer/lib/handle-geometry'
import { cn } from '@/lib/utils'

interface AgentComposerProps {
  wsId: string
  chatId: string
  activity: AgentActivity
  providerLabel: string
  /** This chat's current provider's own declared levels, for the permission
   *  switcher a permission choice offers. Undefined/empty hides the switcher. */
  permissionLevels?: PermissionLevel[]

  live: boolean
  working: boolean
  compacting: boolean
  /** A prompt has been dispatched but the ledger has not yet proven it delivered. */
  sending: boolean
  submitUnavailable: boolean
  terminalWait?: AgentTerminalWait
  /** The pane's own revive attempt, for a chat that is not live. */
  revival?: ComposerRevival
  haltedMessage?: string
  haltedResetsAt?: string
  canStop: boolean

  draft: string
  fieldHeight: number
  slashOpen: boolean
  onDraftChange: (value: string) => void
  onHeightChange: (height: number) => void
  onKeyDown: (event: KeyboardEvent<HTMLDivElement>, readMarkdown: () => string) => void
  onSend: () => void
  onStop: () => void
  onOpenTerminal: () => void
  /** The manual retry for a revive that already gave up. */
  onRevive?: () => void
  /** Bumped when the draft is set from OUTSIDE the box, to remount it. */
  draftSeed: number
  /** The text that seed carries — see the note on `seed` in the view. */
  seedText: string
}

/**
 * THE BAR.
 *
 * One 38px slot with exactly one occupant, resolved by `resolveComposerState`.
 * It is an input when you can talk, and it is the question, the permission, or
 * the reason you cannot, when you cannot — never an input rendered dead beneath
 * something else.
 */
export function AgentComposer(props: AgentComposerProps) {
  const state = resolveComposerState({
    live: props.live,
    revival: props.revival,
    submitUnavailable: props.submitUnavailable,
    terminalWait: props.terminalWait,
    compacting: props.compacting,
    activity: props.activity,
    haltedMessage: props.haltedMessage,
    haltedResetsAt: props.haltedResetsAt,
  })

  switch (state.kind) {
    case 'signpost':
      return (
        <ComposerSignpost
          reason={state.reason}
          message={state.message}
          onOpenTerminal={props.onOpenTerminal}
          onRevive={props.onRevive}
        />
      )
    case 'choice':
      return (
        <ComposerChoice
          wsId={props.wsId}
          chatId={props.chatId}
          activity={props.activity}
          choice={state.choice}
          providerLabel={props.providerLabel}
          permissionLevels={props.permissionLevels}
          onOpenTerminal={props.onOpenTerminal}
        />
      )
    case 'halted':
      return <ComposerHalted message={state.message} resetsAt={state.resetsAt} />
    case 'compacting':
    case 'input': {
      // Compaction does not take the box away — it queues what is typed into it,
      // which is exactly what a busy turn does. The placeholder is the whole
      // difference, because a disabled field here would lose a thought somebody
      // is already halfway through writing.
      const placeholder =
        state.kind === 'compacting'
          ? 'Compacting… your message will be queued'
          : props.working
            ? 'Queue a message…'
            : 'Message the agent…'
      return (
        <div className={cn('pill', isMultiline(props.fieldHeight) && 'multi')}>
          <ComposerField
            key={props.draftSeed}
            initialValue={props.seedText}
            placeholder={placeholder}
            expanded={props.slashOpen}
            controls={props.slashOpen ? 'agent-skill-picker' : undefined}
            onChange={props.onDraftChange}
            onKeyDown={props.onKeyDown}
            onHeightChange={props.onHeightChange}
          />
          <ComposerHandle
            fieldHeight={props.fieldHeight}
            hasText={props.draft.trim().length > 0}
            working={props.working}
            canStop={props.canStop}
            sending={props.sending}
            onSend={props.onSend}
            onStop={props.onStop}
          />
        </div>
      )
    }
  }
}
