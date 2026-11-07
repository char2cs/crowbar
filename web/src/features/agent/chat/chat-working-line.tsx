import { WorkingLine } from '@/features/agent/activity/working-line'
import { useWorkingLineFacts } from '@/features/agent/chat/use-composer-facts'

/** `chatId`'s working line, reading the turn itself: what it says changes with every
 *  tool and token, and the chat view around it need not render for that. */
export function ChatWorkingLine({ chatId, connecting }: { chatId: string; connecting?: string }) {
  return <WorkingLine {...useWorkingLineFacts(chatId)} connecting={connecting} />
}
