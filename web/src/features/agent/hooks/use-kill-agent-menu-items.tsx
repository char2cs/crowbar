import { Skull } from '@phosphor-icons/react'
import type { ContextMenuItem } from '@/components/ui/context-menu'
import { confirmAndKillAgent } from '@/features/agent/lib/kill-agent'
import { useActiveSessionsStore } from '@/lib/store/active-sessions'

const DESTRUCTIVE =
  'text-destructive data-highlighted:bg-destructive/10 data-highlighted:text-destructive dark:data-highlighted:bg-destructive/20'

/**
 * The menu entry that ends the agent running on `chatId`, or nothing when none
 * is. Closing a chat only detaches from it, so this is the one way to stop it.
 */
export function useKillAgentMenuItems(chatId: string | undefined): ContextMenuItem[] {
  const session = useActiveSessionsStore((s) => (chatId ? s.sessions[chatId] : undefined))
  const attached = useActiveSessionsStore((s) => (chatId ? (s.attached[chatId]?.count ?? 0) : 0))
  if (!session) return []
  return [
    { id: 'kill-agent-separator', separator: true, label: '', onClick: () => {} },
    {
      id: 'kill-agent',
      label: 'Kill agent',
      icon: <Skull />,
      className: DESTRUCTIVE,
      onClick: () => void confirmAndKillAgent({ ...session, attached }),
    },
  ]
}
