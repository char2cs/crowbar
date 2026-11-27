import type { ComponentProps, ReactNode, Ref } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { PlanItem } from '@/features/agent/api/chat-rows'
import type { FailureActions } from '@/features/agent/transcript/lib/marker-failure'
import { BottomTray } from '@/features/agent/trays/bottom-tray'
import { TopTray } from '@/features/agent/trays/top-tray'
import { useModelHot } from '@/features/agent/trays/use-model-hot'
import { AgentComposer } from '@/features/agent/composer/agent-composer'
import { ProviderBar } from '@/features/agent/controls/provider-bar'
import { DockStack } from '@/features/agent/chat/dock-stack'

interface ChatDockProps {
  wsId: string
  chatId: string
  blank: boolean
  dockRef: Ref<HTMLDivElement>
  plan: PlanItem[] | undefined
  providers: AgentProvider[]
  failureActions: FailureActions
  onStopShell?: (shellId: string) => void
  onEditPrompt: (text: string) => void
  pickers: ReactNode
  note?: string
  error: string
  composer: Omit<ComponentProps<typeof AgentComposer>, 'wsId' | 'chatId' | 'placement'>
  footer: ComponentProps<typeof ProviderBar>
}

export function ChatDock({
  wsId,
  chatId,
  blank,
  dockRef,
  plan,
  providers,
  failureActions,
  onStopShell,
  onEditPrompt,
  pickers,
  note,
  error,
  composer,
  footer,
}: ChatDockProps) {
  const modelHot = useModelHot(chatId)
  return (
    <DockStack
      blank={blank}
      dockRef={dockRef}
      topTray={
        !blank && (
          <TopTray
            chatId={chatId}
            plan={plan}
            providers={providers}
            actions={failureActions}
            onEdit={onEditPrompt}
          />
        )
      }
      pickers={!blank && pickers}
      notes={
        note && (
          <p className="meta composer-note" data-testid="agent-session-note">
            {note}
          </p>
        )
      }
      pill={
        <AgentComposer
          wsId={wsId}
          chatId={chatId}
          placement={blank ? 'document' : 'dock'}
          {...composer}
        />
      }
      bottomTray={!blank && <BottomTray chatId={chatId} onStopShell={onStopShell} />}
      footer={!blank && <ProviderBar {...footer} modelHot={modelHot} />}
      alert={
        error && (
          <p className="meta composer-note" role="alert">
            {error}
          </p>
        )
      }
    />
  )
}
