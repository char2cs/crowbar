import { stopShell } from '@/features/agent/api/agent-chat-commands'
import { report } from '@/lib/errors/report'

/** Ending a shell, offered only where the provider can be asked and the chat is on its
 *  own surface with a live agent; a refusal is said, not thrown at the page. */
export function offeredStopShell(wsId: string, chatId: string, offered: boolean) {
  if (!offered) return undefined
  return (shellId: string) =>
    void stopShell(wsId, chatId, shellId).catch((error: unknown) =>
      report(error, { action: 'Couldn’t end the shell' }),
    )
}
