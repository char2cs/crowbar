import { ApiError } from '@/lib/errors/api-error'
import { UNREACHED, userWords } from '@/lib/errors/report'
import { toast } from '@/features/window/stores/toast-store'
import { toastHostFailure } from '@/features/hosts/lib/host-failure-notice'

const CLI_NOT_FOUND = 424

const CHAT_BUSY = 'chat_busy'

const SESSION_LOST = 'session_lost'

interface SpawnErrorNotice {
  message: string
  description: string
}

function describeSpawnError(err: unknown, providerName: string, action: string): SpawnErrorNotice {
  if (err instanceof ApiError && err.status === CLI_NOT_FOUND) {
    return {
      message: `${providerName} isn’t installed`,
      description:
        `Crowbar couldn’t find the ${providerName} command on this machine. ` +
        `Install it and make sure it’s on your PATH, then try again.`,
    }
  }
  if (err instanceof ApiError && err.problem.code === CHAT_BUSY) {
    return {
      message: `Can’t ${action} ${providerName} while the chat is working`,
      description: 'Wait for the current turn to finish, or stop it, then try again.',
    }
  }
  if (err instanceof ApiError && err.problem.code === SESSION_LOST) {
    return {
      message: 'The session was lost',
      description: 'Start a new conversation to continue.',
    }
  }
  return {
    message: `Couldn’t ${action} ${providerName} chat`,
    description: userWords(err, UNREACHED) ?? UNREACHED,
  }
}

export function toastSpawnFailure(err: unknown, providerName: string, action: string): void {
  console.error(`Failed to ${action} agent chat:`, err)
  if (toastHostFailure(err)) return
  const { message, description } = describeSpawnError(err, providerName, action)
  toast.error(message, description)
}
