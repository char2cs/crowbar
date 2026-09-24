import type { AgentProvider } from '@/features/agent/api/agent-api'
import { AgentSelectionPicker } from '@/features/agent/controls/agent-selection-picker'
import { ViewSwitcher } from '@/features/agent/controls/view-switcher'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

export interface SelectionClusterProps {
  provider?: AgentProvider
  providers: AgentProvider[]
  model: string
  effort: string
  /** See AgentSelectionPicker's own doc — display only, forwarded untouched. */
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

/**
 * What this chat RUNS AS: which agent, which model, at what effort, on which
 * face of the provider.
 *
 * One cluster because all three answer the same question, which is why they sit
 * closer to each other than to anything else on their row. It exists as its own
 * component because a chat shows it in two places that are not variants of one
 * another — the conversation's underbar, and the blank document's floating
 * handle — and a chat that named its model differently in the two would be
 * describing two different chats.
 */
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
  return (
    <span className="selpos">
      {/* Provider + model + effort as one merged control — picking a model
          already picks its provider, so there is nothing left to split into
          a separate provider chip ahead of it. Stays interactive after the
          chat has launched too, so the user can still switch — it just shows
          what the live runner actually resolved to (AgentChatView passes the
          launch* values once live), not only the sticky request. */}
      <AgentSelectionPicker
        provider={provider}
        providers={providers}
        model={model}
        effort={effort}
        reportedModel={reportedModel}
        disabled={switchDisabled}
        onSelectionChange={onSelectionChange}
        // "Default" is display only, the honest "provider decides" — never
        // baked into `model`/`effort` themselves, which stay '' when unset
        // all the way up through AgentChatView. See AgentSelectionPicker's
        // own doc on `unsetLabel`.
        unsetLabel="Default"
      />
      {showSwitcher && <span className="sep" />}
      {showSwitcher && (
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
