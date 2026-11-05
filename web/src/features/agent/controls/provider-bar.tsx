import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { AgentContextGauge } from '@/features/agent/controls/context-gauge'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import { IS_MOBILE } from '@/utils/platform'

interface ProviderBarProps {
  provider?: AgentProvider
  providers?: AgentProvider[]
  /** A turn is in flight, or a switch is already running. */
  switchDisabled?: boolean
  model: string
  effort: string
  reportedModel?: string
  telemetry: AgentTelemetry | null
  presentation: ChatPresentation
  splitEnabled: boolean
  handoverBlocked?: boolean
  onSelectionChange: (provider: string, model: string, effort: string) => void
  onSelectPresentation: (next: ChatPresentation) => void
  /** Exactly one switcher per pane: both surfaces stay mounted, so an
   *  unconditional one would linger hidden behind the terminal. */
  showSwitcher?: boolean
  /** Prompts waiting behind the running turn. 0 draws nothing. */
  queued?: number
  onCompact?: () => void
}

/** Every control renders nothing when the provider declares nothing — never a
 *  disabled control. `.underbar .left`'s negative margin aligns it to the composer. */
export function ProviderBar({
  provider,
  providers,
  switchDisabled,
  model,
  effort,
  reportedModel,
  telemetry,
  presentation,
  splitEnabled,
  handoverBlocked,
  onSelectionChange,
  onSelectPresentation,
  showSwitcher,
  queued = 0,
  onCompact,
}: ProviderBarProps) {
  return (
    <div className="underbar" data-touch={IS_MOBILE || undefined}>
      <div className="left">
        <SelectionCluster
          provider={provider}
          providers={providers ?? []}
          model={model}
          effort={effort}
          reportedModel={reportedModel}
          presentation={presentation}
          splitEnabled={splitEnabled}
          showSwitcher={showSwitcher}
          handoverBlocked={handoverBlocked}
          switchDisabled={switchDisabled}
          onSelectionChange={onSelectionChange}
          onSelectPresentation={onSelectPresentation}
        />
      </div>
      <div className="right">
        {queued > 0 && <span>{queued} queued</span>}
        {/* The phone draws its meter natively, beside the IDE button (use-native-context). */}
        {!IS_MOBILE && <AgentContextGauge telemetry={telemetry} onCompact={onCompact} />}
      </div>
    </div>
  )
}
