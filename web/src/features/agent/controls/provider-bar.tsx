import { LayersIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { AgentProvider, AgentTelemetry } from '@/features/agent/api/agent-api'
import { AgentContextGauge } from '@/features/agent/controls/context-gauge'
import { AgentModelPicker } from '@/features/agent/controls/model-picker'
import { ViewSwitcher } from '@/features/agent/controls/view-switcher'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

interface ProviderBarProps {
  wsId: string
  chatId: string
  provider?: AgentProvider
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
  /** The empty state boxes the controls instead of the input. */
  boxed?: boolean
  /** A compaction is running right now. */
  compacting?: boolean
  onCompact?: () => void
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
  model,
  effort,
  telemetry,
  presentation,
  splitEnabled,
  handoverBlocked,
  onSelectionChange,
  onSelectPresentation,
  boxed,
  showSwitcher,
  compacting,
  onCompact,
}: ProviderBarProps) {
  return (
    <div className={boxed ? 'underbar boxed' : 'underbar'}>
      <div className="left">
        <AgentModelPicker
          wsId={wsId}
          chatId={chatId}
          provider={provider}
          model={model}
          effort={effort}
          onSelectionChange={onSelectionChange}
        />
        {/* Compaction is the provider's, not Crowbar's — Crowbar cannot compact
            anything itself, it can only ask. Absent entirely for a provider that
            declares no gesture, which is the house rule and not a special case. */}
        {provider?.compaction && onCompact && (
          <Button
            size="xs"
            variant="ghost"
            disabled={compacting}
            aria-label="Compact this conversation"
            tooltip="Ask the provider to compact its context"
            onClick={onCompact}
          >
            <LayersIcon />
            {compacting ? 'Compacting…' : 'Compact'}
          </Button>
        )}
        {showSwitcher && (
          <ViewSwitcher
            presentation={presentation}
            splitEnabled={splitEnabled}
            handoverBlocked={handoverBlocked}
            onSelect={onSelectPresentation}
          />
        )}
      </div>
      <div className="right">
        <AgentContextGauge telemetry={telemetry} />
      </div>
    </div>
  )
}
