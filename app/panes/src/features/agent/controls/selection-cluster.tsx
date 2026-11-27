import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { AgentSelectionPicker } from '@/features/agent/controls/agent-selection-picker'
import {
  PermissionLevelChip,
  type PermissionTarget,
} from '@/features/agent/composer/permission-level-switcher'
import { IS_MOBILE } from '@/utils/platform'

export interface SelectionClusterProps {
  provider?: AgentProvider
  providers: AgentProvider[]
  model: string
  effort: string
  reportedModel?: string
  switchDisabled?: boolean
  hot?: boolean
  permission?: PermissionTarget
  onSelectionChange: (provider: string, model: string, effort: string) => void
}

export function SelectionCluster({
  provider,
  providers,
  model,
  effort,
  reportedModel,
  switchDisabled,
  hot,
  permission,
  onSelectionChange,
}: SelectionClusterProps) {
  return (
    <span className="selpos">
      <AgentSelectionPicker
        provider={provider}
        providers={providers}
        model={model}
        effort={effort}
        reportedModel={reportedModel}
        disabled={switchDisabled}
        hot={hot}
        permission={permission}
        onSelectionChange={onSelectionChange}
        unsetLabel="Default"
        modelUnsetLabel="Model"
      />
      {!IS_MOBILE && permission && <PermissionLevelChip target={permission} />}
    </span>
  )
}
