import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { AgentContextGauge } from '@/features/agent/controls/context-gauge'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
import {
  PermissionLevelChip,
  type PermissionTarget,
} from '@/features/agent/composer/permission-level-switcher'
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
  /** The chat's permission level and the levels its provider offers: a chip on the desktop,
   *  a section of the model picker sheet on the phone. */
  permission?: PermissionTarget
  /** The conversation is being folded: named before the pill. The input stays usable. */
  compacting?: boolean
  onCompact?: () => void
}

/** The footer line under the pill. Every control renders nothing when the provider declares
 *  nothing — never a disabled control. */
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
  permission,
  compacting,
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
          permission={permission}
          onSelectionChange={onSelectionChange}
          onSelectPresentation={onSelectPresentation}
        />
        {!IS_MOBILE && permission && <PermissionLevelChip {...permission} />}
      </div>
      <div className="right">
        {compacting && <span className="ctx-compacting">Compacting…</span>}
        <AgentContextGauge telemetry={telemetry} onCompact={onCompact} />
      </div>
    </div>
  )
}
