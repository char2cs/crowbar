import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { AgentSelectionPicker } from '@/features/agent/controls/agent-selection-picker'
import { ViewSwitcher } from '@/features/agent/controls/view-switcher'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import { IS_MOBILE } from '@/utils/platform'

export interface SelectionClusterProps {
  provider?: AgentProvider
  providers: AgentProvider[]
  model: string
  effort: string
  reportedModel?: string
  presentation: ChatPresentation
  splitEnabled: boolean
  /** Draw the surface switcher. Exactly one exists on a pane. */
  showSwitcher?: boolean
  handoverBlocked?: boolean
  switchDisabled?: boolean
  onSelectionChange: (provider: string, model: string, effort: string) => void
  onSelectPresentation: (next: ChatPresentation) => void
}

/** Shared by the underbar and the blank document's handle so a chat never names
 *  its model differently in the two. */
export function SelectionCluster({
  provider,
  providers,
  model,
  effort,
  reportedModel,
  presentation,
  splitEnabled,
  showSwitcher,
  handoverBlocked,
  switchDisabled,
  onSelectionChange,
  onSelectPresentation,
}: SelectionClusterProps) {
  // The phone has one surface, the transcript (useChatPresentation pins it there).
  const switcher = showSwitcher && !IS_MOBILE
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
        onSelectionChange={onSelectionChange}
        // Display only: `model`/`effort` stay '' when unset.
        unsetLabel="Default"
      />
      {switcher && <span className="sep" />}
      {switcher && (
        <ViewSwitcher
          presentation={presentation}
          splitEnabled={splitEnabled}
          handoverBlocked={handoverBlocked}
          onSelect={onSelectPresentation}
        />
      )}
    </span>
  )
}
