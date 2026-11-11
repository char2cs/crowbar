import type { ComponentProps, ReactNode, Ref } from 'react'
import type { PlanItem } from '@/features/agent/api/chat-rows'
import { BottomTray } from '@/features/agent/trays/bottom-tray'
import { TopTray } from '@/features/agent/trays/top-tray'
import { AgentComposer } from '@/features/agent/composer/agent-composer'
import { ProviderBar } from '@/features/agent/controls/provider-bar'
import { DockStack } from '@/features/agent/chat/dock-stack'

interface ChatDockProps {
  wsId: string
  chatId: string
  /** Nothing said yet: the composer is the empty document and the footer is its own. */
  blank: boolean
  dockRef: Ref<HTMLDivElement>
  plan: PlanItem[] | undefined
  /** Puts an unsent message's text back in the composer. */
  onEditPrompt: (text: string) => void
  pickers: ReactNode
  /** Why the chat cannot take a prompt yet. */
  note?: string
  /** A send-time error. */
  error: string
  composer: Omit<ComponentProps<typeof AgentComposer>, 'wsId' | 'chatId' | 'placement'>
  footer: ComponentProps<typeof ProviderBar>
}

/** What stands around the composer in a live chat, wired into the dock's slots. */
export function ChatDock({
  wsId,
  chatId,
  blank,
  dockRef,
  plan,
  onEditPrompt,
  pickers,
  note,
  error,
  composer,
  footer,
}: ChatDockProps) {
  return (
    <DockStack
      blank={blank}
      dockRef={dockRef}
      topTray={!blank && <TopTray chatId={chatId} plan={plan} onEdit={onEditPrompt} />}
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
      bottomTray={!blank && <BottomTray chatId={chatId} />}
      footer={!blank && <ProviderBar {...footer} />}
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
