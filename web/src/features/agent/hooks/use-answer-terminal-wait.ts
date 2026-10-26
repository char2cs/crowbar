import { useCallback } from 'react'
import { answerTerminalWait } from '@/features/agent/api/agent-api'
import { actionErrorMessage } from '@/features/hosts/lib/host-failure-notice'
import { toast } from '@/features/window/stores/toast-store'

/** Sends a terminal prompt's declared answer; a failure is toasted, a success
 *  needs nothing — the wait clears when the daemon sees the prompt gone. */
export function useAnswerTerminalWait(wsId: string, chatId: string) {
  return useCallback(
    (answerId: string) => {
      answerTerminalWait(wsId, chatId, answerId).catch((err: unknown) => {
        toast.error("Couldn't send that answer", actionErrorMessage(err, String(err)))
      })
    },
    [wsId, chatId],
  )
}
