import { problemOf } from '@/lib/errors/problem'
import { toastManager } from '@/lib/toast-manager'
import { reportDaemonRefusal } from '@/lib/ws/connection-store'

/** What the user was doing when it failed; `key` dedupes repeats of one failure. */
interface ReportContext {
  action: string
  key?: string
}

// A cancelled request or Monaco provider call is a superseded one, not a failure.
function isCancellation(error: unknown): boolean {
  if (!(error instanceof Error) && !(error instanceof DOMException)) return false
  return error.name === 'AbortError' || error.name === 'Canceled'
}

/** Tells the user an action failed and why, in the daemon's own words. Exported
 *  when its first caller lands; until then only rejections reach it. */
function report(error: unknown, context: ReportContext): void {
  if (isCancellation(error)) return
  const problem = problemOf(error)
  // A migrating or halted daemon has its own screen; a toast per refused call would bury it.
  if (problem.code === 'daemon_migrating' || problem.code === 'daemon_halted') {
    reportDaemonRefusal(problem)
    return
  }
  toastManager.add({
    id: context.key ?? context.action,
    type: 'error',
    title: context.action,
    description: problem.message,
  })
}

/** Reports every promise rejection nothing handled; returns the uninstaller. */
export function installGlobalReporting(target: Window = window): () => void {
  const onRejection = (event: Event) =>
    report((event as PromiseRejectionEvent).reason, { action: 'Something went wrong' })
  target.addEventListener('unhandledrejection', onRejection)
  return () => target.removeEventListener('unhandledrejection', onRejection)
}
