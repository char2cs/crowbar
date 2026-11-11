import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { AgentSelectionPicker } from '@/features/agent/controls/agent-selection-picker'
import type { PermissionTarget } from '@/features/agent/composer/permission-level-switcher'

export interface SelectionClusterProps {
  provider?: AgentProvider
  providers: AgentProvider[]
  model: string
  effort: string
  reportedModel?: string
  switchDisabled?: boolean
  /** Offered inside the model picker's phone sheet. */
  permission?: PermissionTarget
  onSelectionChange: (provider: string, model: string, effort: string) => void
}

/** Shared by the underbar and the blank document's handle so a chat never names
 *  its model differently in the two. */
export function SelectionCluster({
  provider,
  providers,
  model,
  effort,
  reportedModel,
  switchDisabled,
  permission,
  onSelectionChange,
}: SelectionClusterProps) {
  return (
    <span className="selpos">
      {/* One merged control: picking a model already picks its provider. */}
      <AgentSelectionPicker
        provider={provider}
        providers={providers}
        model={model}
        effort={effort}
        reportedModel={reportedModel}
        disabled={switchDisabled}
        permission={permission}
        onSelectionChange={onSelectionChange}
        // Display only: `model`/`effort` stay '' when unset.
        unsetLabel="Default"
      />
    </span>
  )
}
