import { useCallback, useEffect, useRef } from 'react'
import { report } from '@/lib/errors/report'
import { UserFacingError } from '@/lib/errors/user-facing-error'
import { stashPrompt } from '@/features/agent/composer/lib/prompt-stash-persistence'
import { stopTurn } from '@/features/agent/stores/chat-outbox'

interface TakeBackDeps {
  wsId: string
  chatId: string
  working: boolean
  putBackInComposer: (text: string) => boolean
  withdraw: () => Promise<string | null> | null
  retract: () => Promise<string | null> | null
}

export function usePromptTakeBack({
  wsId,
  chatId,
  working,
  putBackInComposer,
  withdraw,
  retract,
}: TakeBackDeps) {
  const shownChat = useRef<string | null>(null)
  useEffect(() => {
    shownChat.current = chatId
    return () => {
      shownChat.current = null
    }
  }, [chatId])

  const putBack = useCallback(
    (taken: Promise<string | null> | null) => {
      const asked = chatId
      void taken?.then((text) => {
        if (text === null) return
        if (shownChat.current === asked && putBackInComposer(text)) return
        if (!stashPrompt(wsId, asked, text)) {
          report(new UserFacingError('The saved prompts are full or unavailable.'), {
            action: 'Couldn’t keep the prompt you took back',
          })
        }
      })
      return taken !== null
    },
    [wsId, chatId, putBackInComposer],
  )

  return {
    stop: () => (working ? void stopTurn(wsId, chatId) : void putBack(withdraw())),
    retract: () => putBack(retract()),
  }
}
