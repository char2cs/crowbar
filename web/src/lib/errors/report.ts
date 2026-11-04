import { hostFailureWords } from '@/features/hosts/lib/host-failure-notice'
import { ApiError } from '@/lib/errors/api-error'
import { OperationFailedError } from '@/lib/errors/operation-failed'
import { problemOf } from '@/lib/errors/problem'
import { UserFacingError } from '@/lib/errors/user-facing-error'
import { toastManager } from '@/lib/toast-manager'
import { reportDaemonRefusal } from '@/lib/ws/connection-store'

/** What the user was doing when it failed. */
interface ReportContext {
  action: string
  /** Why, when the throw is not an Error and so carries no words of its own. */
  fallback?: string
}

const UNEXPECTED = 'Something went wrong'

/** What the user reads for a client bug, whose own text stays in the log. */
const UNEXPLAINED = 'Something went wrong — details are in the log'

/** No daemon answered to word the failure: a request that reached none, or a non-Error throw. */
export const UNREACHED = 'Crowbar could not reach the daemon — try again.'

// A cancelled request or Monaco provider call is a superseded one, not a failure.
function isCancellation(error: unknown): boolean {
  if (!(error instanceof Error) && !(error instanceof DOMException)) return false
  return error.name === 'AbortError' || error.name === 'Canceled'
}

function isWorded(error: unknown): error is Error {
  return (
    error instanceof ApiError ||
    error instanceof OperationFailedError ||
    error instanceof UserFacingError
  )
}

/** Why `error` happened, in words for the user: the daemon's, or a sentence written for
 *  them; a client bug says only that it is in the log. A non-Error carries `fallback`. */
export function userWords(error: unknown, fallback?: string): string | undefined {
  if (isWorded(error)) return problemOf(error).message
  return error instanceof Error ? UNEXPLAINED : fallback
}

/** Tells the user an action failed and why: its host's trouble when the host is why, else
 *  the daemon's own words. */
export function report(error: unknown, context: ReportContext): void {
  // A failed operation was already toasted by the operations watcher.
  if (isCancellation(error) || error instanceof OperationFailedError) return
  // A migrating or halted daemon has its own screen; a toast per refused call would bury it.
  if (reportDaemonRefusal(problemOf(error))) return
  const hostWords = hostFailureWords(error)
  if (hostWords === null && error instanceof Error && !isWorded(error)) {
    console.error(`${context.action}:`, error)
  }
  const description = hostWords ?? userWords(error, context.fallback)
  toastManager.add({ id: context.action, type: 'error', title: context.action, description })
}

/** Reports every promise rejection nothing handled; returns the uninstaller. */
export function installGlobalReporting(target: Window = window): () => void {
  const onRejection = (event: Event) => {
    const reason: unknown = (event as PromiseRejectionEvent).reason
    // report() logs a client Error itself; a thrown non-Error is logged here.
    if (!(reason instanceof Error) && !isCancellation(reason)) {
      console.error('unhandled rejection:', reason)
    }
    report(reason, { action: UNEXPECTED, fallback: UNEXPLAINED })
  }
  target.addEventListener('unhandledrejection', onRejection)
  return () => target.removeEventListener('unhandledrejection', onRejection)
}
