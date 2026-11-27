import type { ReactNode } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { AgentContextGauge } from '@/features/agent/controls/context-gauge'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
import type { PermissionTarget } from '@/features/agent/composer/permission-level-switcher'
import { IS_MOBILE } from '@/utils/platform'

interface ProviderBarProps {
  provider?: AgentProvider
  providers?: AgentProvider[]
  switchDisabled?: boolean
  model: string
  effort: string
  reportedModel?: string
  telemetry: AgentTelemetry | null
  onSelectionChange: (provider: string, model: string, effort: string) => void
  permission?: PermissionTarget
  compacting?: boolean
  modelHot?: boolean
  onCompact?: () => void
  viewSwitch?: ReactNode
}

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
  compacting,
  modelHot,
  onCompact,
  viewSwitch,
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
          hot={modelHot}
          onSelectionChange={onSelectionChange}
        />
      </div>
      {viewSwitch && <div className="mid">{viewSwitch}</div>}
      <div className="right">
        {compacting && <span className="ctx-compacting">Compacting…</span>}
        <AgentContextGauge telemetry={telemetry} compacting={compacting} onCompact={onCompact} />
      </div>
    </div>
  )
}
