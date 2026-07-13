import { ApiError } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'

/** HTTP 424 Failed Dependency — the daemon could not find the provider's vendor CLI
 *  on this machine (not installed, or not on the PATH the daemon actually has). It is
 *  the one spawn failure the user can fix, so it gets its own words. */
const CLI_NOT_FOUND = 424

export interface SpawnErrorNotice {
  message: string
  description: string
}

/** Turn a failed spawn into what to SAY about it.
 *
 *  Pure and separately tested: every spawn route (new chat, resume, provider switch)
 *  goes through the same daemon path and so fails the same ways, and all three used to
 *  swallow the failure into console.error — which is exactly how a packaged build
 *  shipped with EVERY chat click doing nothing at all, with no way for the user to
 *  discover that the CLI simply was not installed.
 *
 *  `action` is the verb, already phrased for the sentence it lands in ("start",
 *  "resume", "switch"). */
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
  const detail = err instanceof Error ? err.message : String(err)
  return {
    message: `Couldn’t ${action} ${providerName} chat`,
    description: detail,
  }
}

/** Report a failed spawn to the user AND to the console — the toast is what they see,
 *  the console line is what a bug report can carry. */
export function toastSpawnFailure(err: unknown, providerName: string, action: string): void {
  const { message, description } = describeSpawnError(err, providerName, action)
  console.error(`Failed to ${action} agent chat:`, err)
  toast.error(message, description)
}
