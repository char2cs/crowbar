import { WorkingLine } from '@/features/agent/activity/working-line'
import { useWorkingLineFacts } from '@/features/agent/chat/use-composer-facts'

export function ChatWorkingLine({ chatId, connecting }: { chatId: string; connecting?: string }) {
  return <WorkingLine {...useWorkingLineFacts(chatId)} connecting={connecting} />
}
