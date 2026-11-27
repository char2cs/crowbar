import { hostFailureWords } from '@/features/hosts/lib/host-failure-notice'
import { ApiError } from '@/lib/errors/api-error'
import { OperationFailedError } from '@/lib/errors/operation-failed'
import { problemOf } from '@/lib/errors/problem'
import { UserFacingError } from '@/lib/errors/user-facing-error'
import { toastManager } from '@/lib/toast-manager'
import { reportDaemonRefusal } from '@/lib/ws/connection-store'

interface ReportContext {
  action: string
  fallback?: string
}

const UNEXPECTED = 'Something went wrong'

const UNEXPLAINED = 'Something went wrong — details are in the log'

export const UNREACHED = 'Crowbar could not reach the daemon — try again.'

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

export function userWords(error: unknown, fallback?: string): string | undefined {
  if (isWorded(error)) return problemOf(error).message
  return error instanceof Error ? UNEXPLAINED : fallback
}

export function report(error: unknown, context: ReportContext): void {
  if (isCancellation(error) || error instanceof OperationFailedError) return
  if (reportDaemonRefusal(problemOf(error))) return
  const hostWords = hostFailureWords(error)
  if (hostWords === null && error instanceof Error && !isWorded(error)) {
    console.error(`${context.action}:`, error)
  }
  const description = hostWords ?? userWords(error, context.fallback)
  toastManager.add({ id: context.action, type: 'error', title: context.action, description })
}

export function installGlobalReporting(target: Window = window): () => void {
  const onRejection = (event: Event) => {
    const reason: unknown = (event as PromiseRejectionEvent).reason
    if (!(reason instanceof Error) && !isCancellation(reason)) {
      console.error('unhandled rejection:', reason)
    }
    report(reason, { action: UNEXPECTED, fallback: UNEXPLAINED })
  }
  target.addEventListener('unhandledrejection', onRejection)
  return () => target.removeEventListener('unhandledrejection', onRejection)
}
