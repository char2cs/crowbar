import { ApiError } from '@/lib/errors/api-error'
import { OperationFailedError } from '@/lib/errors/operation-failed'
import { problemOf } from '@/lib/errors/problem'
import { toastManager } from '@/lib/toast-manager'
import { reportDaemonRefusal } from '@/lib/ws/connection-store'

/** What the user was doing when it failed. */
interface ReportContext {
  action: string
}

const UNEXPECTED = 'Something went wrong'

// A cancelled request or Monaco provider call is a superseded one, not a failure.
function isCancellation(error: unknown): boolean {
  if (!(error instanceof Error) && !(error instanceof DOMException)) return false
  return error.name === 'AbortError' || error.name === 'Canceled'
}

/** Tells the user an action failed and why, in the daemon's own words. */
function report(error: unknown, context: ReportContext): void {
  // A failed operation was already toasted by the operations watcher.
  if (isCancellation(error) || error instanceof OperationFailedError) return
  const problem = problemOf(error)
  // A migrating or halted daemon has its own screen; a toast per refused call would bury it.
  if (problem.code === 'daemon_migrating' || problem.code === 'daemon_halted') {
    reportDaemonRefusal(problem)
    return
  }
  toastManager.add({
    id: context.action,
    type: 'error',
    title: context.action,
    description: problem.message,
  })
}

/** Reports every promise rejection nothing handled; returns the uninstaller. */
export function installGlobalReporting(target: Window = window): () => void {
  const onRejection = (event: Event) => {
    const reason: unknown = (event as PromiseRejectionEvent).reason
    if (
      reason instanceof ApiError ||
      reason instanceof OperationFailedError ||
      isCancellation(reason)
    ) {
      report(reason, { action: UNEXPECTED })
      return
    }
    // A client bug's own text means nothing to the user; it stays in the console.
    console.error('unhandled rejection:', reason)
    report(new Error(UNEXPECTED), { action: UNEXPECTED })
  }
  target.addEventListener('unhandledrejection', onRejection)
  return () => target.removeEventListener('unhandledrejection', onRejection)
}
