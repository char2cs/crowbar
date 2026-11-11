import type { ComponentProps, ReactNode, Ref } from 'react'
import { SubagentShelf } from '@/features/agent/activity/subagent-shelf'
import { TodoStrip } from '@/features/agent/activity/todo-strip'
import { AgentComposer } from '@/features/agent/composer/agent-composer'
import { ProviderBar } from '@/features/agent/controls/provider-bar'
import { DockStack } from '@/features/agent/chat/dock-stack'

interface ChatDockProps {
  wsId: string
  chatId: string
  /** Nothing said yet: the composer is the empty document and the footer is its own. */
  blank: boolean
  dockRef: Ref<HTMLDivElement>
  plan: ComponentProps<typeof TodoStrip>['plan']
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
      topTray={
        !blank && (
          <>
            <TodoStrip plan={plan} />
            <SubagentShelf chatId={chatId} />
          </>
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
