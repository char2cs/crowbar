import type { AgentProvider, AgentTelemetry } from '@/features/agent/api/agent-api'
import { AgentContextGauge } from '@/features/agent/controls/context-gauge'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

interface ProviderBarProps {
  wsId: string
  chatId: string
  provider?: AgentProvider
  /** Every provider Crowbar knows, for the identity chip's other groups. */
  providers?: AgentProvider[]
  /** Move this chat to another provider. Absent means handover is not offered. */
  onSwitchProvider?: (providerId: string) => Promise<boolean>
  /** A turn is in flight, or a switch is already running. */
  switchDisabled?: boolean
  model: string
  effort: string
  telemetry: AgentTelemetry | null
  presentation: ChatPresentation
  splitEnabled: boolean
  handoverBlocked?: boolean
  onSelectionChange: (model: string, effort: string) => void
  onSelectPresentation: (next: ChatPresentation) => void
  /**
   * Draw the surface switcher here.
   *
   * EXACTLY ONE of these exists on a pane. Both surfaces stay mounted — the
   * hidden one keeps its PTY and its scrollback — so a switcher rendered
   * unconditionally here is still in the document while the terminal is in
   * front, where it is invisible to a user and ambiguous to everything else.
   * The pane's strip carries it on the other two surfaces.
   */
  showSwitcher?: boolean
  /** Prompts waiting behind the running turn. 0 draws nothing. */
  queued?: number
}

/**
 * The row under the composer: what this chat RUNS AS on the left, what it has
 * SPENT on the right.
 *
 * Every control here is provider-declared and every one of them renders nothing
 * at all when the provider declares nothing, so the row collapses to whichever
 * half exists. That is the house rule and it is not negotiable: a provider with
 * no model catalogue gets NO PICKER, never a disabled one. A greyed-out control
 * says "broken"; absence says "this provider does not offer that".
 *
 * It is aligned to the composer, not to the pane — the leading chip's glyph
 * lines up with the pill's first character, which is why `.underbar .left`
 * carries a negative margin rather than the row carrying padding.
 */
export function ProviderBar({
  wsId,
  chatId,
  provider,
  providers,
  onSwitchProvider,
  switchDisabled,
  model,
  effort,
  telemetry,
  presentation,
  splitEnabled,
  handoverBlocked,
  onSelectionChange,
  onSelectPresentation,
  showSwitcher,
  queued = 0,
}: ProviderBarProps) {
  return (
    <div className="underbar">
      {/* Left: what this chat RUNS AS — the model, its effort, and which face of
          the provider you are looking at. One cluster, because all three answer
          the same question and the eye should not have to group them. */}
      <div className="left">
        <SelectionCluster
          wsId={wsId}
          chatId={chatId}
          provider={provider}
          providers={providers ?? []}
          model={model}
          effort={effort}
          presentation={presentation}
          splitEnabled={splitEnabled}
          showSwitcher={showSwitcher}
          handoverBlocked={handoverBlocked}
          switchDisabled={switchDisabled}
          onSwitchProvider={onSwitchProvider}
          onSelectionChange={onSelectionChange}
          onSelectPresentation={onSelectPresentation}
        />
      </div>
      {/* Right: what this chat has SPENT, and the one gesture that spends less. */}
      <div className="right">
        {queued > 0 && <span>{queued} queued</span>}
        <AgentContextGauge telemetry={telemetry} />
      </div>
    </div>
  )
}
