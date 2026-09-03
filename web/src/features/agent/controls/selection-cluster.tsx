import type { AgentProvider } from '@/features/agent/api/agent-api'
import { AgentModelPicker } from '@/features/agent/controls/model-picker'
import { AgentProviderPicker } from '@/features/agent/controls/provider-picker'
import { ViewSwitcher } from '@/features/agent/controls/view-switcher'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

export interface SelectionClusterProps {
  wsId: string
  chatId: string
  provider?: AgentProvider
  providers: AgentProvider[]
  model: string
  effort: string
  presentation: ChatPresentation
  splitEnabled: boolean
  /** Draw the surface switcher. Exactly one exists on a pane. */
  showSwitcher?: boolean
  handoverBlocked?: boolean
  switchDisabled?: boolean
  onSwitchProvider?: (providerId: string) => Promise<boolean>
  onSelectionChange: (model: string, effort: string) => void
  onSelectPresentation: (next: ChatPresentation) => void
}

/**
 * What this chat RUNS AS: which agent, which model, at what effort, on which
 * face of the provider.
 *
 * One cluster because all four answer the same question, which is why they sit
 * closer to each other than to anything else on their row. It exists as its own
 * component because a chat shows it in two places that are not variants of one
 * another — the conversation's underbar, and the blank document's floating
 * handle — and a chat that named its model differently in the two would be
 * describing two different chats.
 */
export function SelectionCluster({
  wsId,
  chatId,
  provider,
  providers,
  model,
  effort,
  presentation,
  splitEnabled,
  showSwitcher,
  handoverBlocked,
  switchDisabled,
  onSwitchProvider,
  onSelectionChange,
  onSelectPresentation,
}: SelectionClusterProps) {
  return (
    <span className="selpos">
      {/* WHOSE CLI, then WHAT IT RUNS AS. In that order because that is the
          order they depend on each other: the catalogue behind the model chip
          belongs to whichever agent this one names. */}
      {onSwitchProvider && (
        <>
          <AgentProviderPicker
            provider={provider}
            providers={providers}
            disabled={switchDisabled}
            onSwitch={(id) => void onSwitchProvider(id)}
          />
          <span className="sep" />
        </>
      )}
      <AgentModelPicker
        wsId={wsId}
        chatId={chatId}
        provider={provider}
        model={model}
        effort={effort}
        onSelectionChange={onSelectionChange}
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
