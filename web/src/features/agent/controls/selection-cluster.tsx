import { ProviderIcon } from '@/components/ui/provider-icon'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { AgentSelectionPicker, effortLabel } from '@/features/agent/controls/agent-selection-picker'
import { ViewSwitcher } from '@/features/agent/controls/view-switcher'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

export interface SelectionClusterProps {
  provider?: AgentProvider
  providers: AgentProvider[]
  model: string
  effort: string
  /** The chat is live: draw model/effort as plain text (what it actually
   *  launched as) instead of the interactive picker — a launch that already
   *  happened is not a choice left to make. */
  readOnly?: boolean
  presentation: ChatPresentation
  splitEnabled: boolean
  /** Draw the surface switcher. Exactly one exists on a pane. */
  showSwitcher?: boolean
  handoverBlocked?: boolean
  switchDisabled?: boolean
  onSelectionChange: (provider: string, model: string, effort: string) => void
  onSelectPresentation: (next: ChatPresentation) => void
}

/** The read-only twin of AgentSelectionPicker's own trigger — same glyph
 *  order (provider, model, effort), no chevron, nothing to click. A provider
 *  with no model/effort catalogue still names itself here (unlike the picker,
 *  which renders nothing for one — there is no pick to offer, but a started
 *  chat still has a provider worth showing). */
function LaunchLabel({
  provider,
  model,
  effort,
}: {
  provider?: AgentProvider
  model: string
  effort: string
}) {
  return (
    <span
      className="chip max-w-56"
      title={`${provider?.displayName ?? 'Agent'} — what this chat launched as.`}
    >
      {provider && <ProviderIcon svg={provider.icon} className="size-3" />}
      {model && <b className="font-semibold text-foreground">{model}</b>}
      {model && effort && <span className="opacity-50">&middot;</span>}
      {effort && <span className="truncate">{effortLabel(effort)}</span>}
      {!model && <span className="truncate">{provider?.displayName ?? 'Agent'}</span>}
    </span>
  )
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
  readOnly,
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
          a separate provider chip ahead of it. Read-only once live: this
          launch already happened, so there is nothing left to pick. */}
      {readOnly ? (
        <LaunchLabel provider={provider} model={model} effort={effort} />
      ) : (
        <AgentSelectionPicker
          provider={provider}
          providers={providers}
          model={model}
          effort={effort}
          disabled={switchDisabled}
          onSelectionChange={onSelectionChange}
        />
      )}
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
