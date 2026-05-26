// Stub
export type DiagnosticSeverity = "error" | "warning" | "info" | "hint"
export interface Diagnostic {
  message: string
  severity: DiagnosticSeverity
  range: { start: { line: number; character: number }; end: { line: number; character: number } }
  /** Flat positional properties used by some Athas decorators */
  line?: number
  endLine?: number
  column?: number
  endColumn?: number
  source?: string
  code?: string | number
}
export type ApplyDiagnosticCodeActionResult = { success: boolean; message?: string }
export interface DiagnosticCodeAction {
  title: string
  kind?: string
  diagnostics?: Diagnostic[]
  edit?: unknown
}
