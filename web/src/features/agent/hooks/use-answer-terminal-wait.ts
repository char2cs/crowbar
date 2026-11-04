import { useCallback } from 'react'
import { answerTerminalWait } from '@/features/agent/api/agent-chat-commands'
import { report } from '@/lib/errors/report'

/** Sends a terminal prompt's declared answer; a failure is reported, a success
 *  needs nothing — the wait clears when the daemon sees the prompt gone. */
export function useAnswerTerminalWait(wsId: string, chatId: string) {
  return useCallback(
    (answerId: string) => {
      answerTerminalWait(wsId, chatId, answerId).catch((err: unknown) =>
        report(err, { action: "Couldn't send that answer" }),
      )
    },
    [wsId, chatId],
  )
}
