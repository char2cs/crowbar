import { ApiError } from '@/lib/errors/api-error'
import { UNREACHED, userWords } from '@/lib/errors/report'
import { toast } from '@/features/window/stores/toast-store'
import { toastHostFailure } from '@/features/hosts/lib/host-failure-notice'

/** HTTP 424 Failed Dependency — the daemon could not find the provider's vendor CLI
 *  on this machine (not installed, or not on the PATH the daemon actually has). It is
 *  the one spawn failure the user can fix, so it gets its own words. */
const CLI_NOT_FOUND = 424

/** The daemon's code for a chat that is working and so refuses the action. */
const CHAT_BUSY = 'chat_busy'

/** The provider no longer has the chat's session: only a new conversation continues it. */
const SESSION_LOST = 'session_lost'

export interface SpawnErrorNotice {
  message: string
  description: string
}

/** Turn a failed spawn into what to SAY about it. Every spawn route (new chat, resume, switch)
 *  fails the same ways through one daemon path, and a swallowed failure hides e.g. a missing CLI.
 *  `action` is the verb as phrased in the sentence ("start", "resume", "switch"). */
export function describeSpawnError(
  err: unknown,
  providerName: string,
  action: string,
): SpawnErrorNotice {
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
  // Never the daemon's own text: it names Crowbar's internals, not what to do.
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

/** Report a failed spawn to the user AND to the console — the toast is what they see,
 *  the console line is what a bug report can carry. */
export function toastSpawnFailure(err: unknown, providerName: string, action: string): void {
  console.error(`Failed to ${action} agent chat:`, err)
  if (toastHostFailure(err)) return
  const { message, description } = describeSpawnError(err, providerName, action)
  toast.error(message, description)
}
