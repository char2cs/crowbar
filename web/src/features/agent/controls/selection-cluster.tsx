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
  /** The model chip is lit: the way on is a different model. */
  hot?: boolean
  /** A chip after the picker on the desktop, a section of the model picker's sheet on the phone. */
  permission?: PermissionTarget
  onSelectionChange: (provider: string, model: string, effort: string) => void
}

/** Shared by the underbar and the blank document's handle so a chat never names its model or
 *  permission level differently in the two. */
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
      {/* One merged control: picking a model already picks its provider. */}
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
        // Display only: `model`/`effort` stay '' when unset.
        unsetLabel="Default"
      />
      {!IS_MOBILE && permission && <PermissionLevelChip target={permission} />}
    </span>
  )
}
