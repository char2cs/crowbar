import { stopShell } from '@/features/agent/api/agent-chat-commands'
import { toast } from '@/features/window/stores/toast-store'
import { ApiError } from '@/lib/errors/api-error'
import { report } from '@/lib/errors/report'

export function offeredStopShell(wsId: string, chatId: string, offered: boolean) {
  if (!offered) return undefined
  return (shellId: string) =>
    void stopShell(wsId, chatId, shellId).catch((error: unknown) => {
      if (error instanceof ApiError && error.problem.code === 'chat_busy') {
        toast.info('The agent is busy', 'End the shell once the current turn finishes.')
        return
      }
      report(error, { action: 'Couldn’t end the shell' })
    })
}
