import { ApiError } from '@/lib/errors/api-error'

/** The daemon's closed code enum (`domain/problem`), same spelling, plus the
 *  desktop proxy's own `daemon_unavailable`. */
export type ProblemCode =
  | 'invalid_argument'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'locked'
  | 'too_large'
  | 'unprocessable'
  | 'failed_dependency'
  | 'internal'
  | 'bad_gateway'
  | 'unavailable'
  | 'timeout'
  | 'chat_busy'
  | 'request_outcome_uncertain'
  | 'request_id_conflict'
  | 'prompt_submit_unsupported'
  | 'live_tui_required'
  | 'catalog_unsupported'
  | 'catalog_timeout'
  | 'catalog_command_unavailable'
  | 'catalog_output_limit'
  | 'catalog_command_failed'
  | 'catalog_malformed_output'
  | 'catalog_superseded'
  | 'host_document_requires_local_transport'
  | 'daemon_state_document'
  | 'work_at_risk'
  | 'id_conflict'
  | 'operation_id_conflict'
  | 'operation_running'
  | 'daemon_migrating'
  | 'daemon_halted'
  | 'daemon_report_unencodable'
  | 'interrupted'
  | 'partial'
  | 'daemon_unavailable'

/** A failure as the client reads it: the daemon's code and message. */
export interface Problem {
  code: ProblemCode
  message: string
}

/** Any thrown value as a Problem, so every surface words a failure the same way. */
export function problemOf(error: unknown): Problem {
  if (error instanceof ApiError) return error.problem
  if (error instanceof Error) return { code: 'internal', message: error.message }
  return { code: 'internal', message: String(error) }
}
