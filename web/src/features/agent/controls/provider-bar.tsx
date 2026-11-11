import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { AgentContextGauge } from '@/features/agent/controls/context-gauge'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
import type {
  PermissionControl,
  PermissionTarget,
} from '@/features/agent/composer/permission-level-switcher'
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
  onSelectionChange: (provider: string, model: string, effort: string) => void
  /** The chat's permission level and the levels its provider offers: a chip on the desktop,
   *  a section of the model picker sheet on the phone. */
  permission?: PermissionTarget
  permissionLevel?: PermissionControl
  /** The conversation is being folded: named before the pill. The input stays usable. */
  compacting?: boolean
  /** A stop another model gets past: the model chip is lit. */
  modelHot?: boolean
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
  onSelectionChange,
  permission,
  permissionLevel,
  compacting,
  modelHot,
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
          switchDisabled={switchDisabled}
          permission={permission}
          permissionLevel={permissionLevel}
          hot={modelHot}
          onSelectionChange={onSelectionChange}
        />
      </div>
      <div className="right">
        {compacting && <span className="ctx-compacting">Compacting…</span>}
        <AgentContextGauge telemetry={telemetry} compacting={compacting} onCompact={onCompact} />
      </div>
    </div>
  )
}
