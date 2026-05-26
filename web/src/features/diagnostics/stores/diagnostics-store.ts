// Stub: diagnostics feature is out of scope for this session.
import { create } from 'zustand'
import { createSelectors } from '@/utils/zustand-selectors'
import type { Diagnostic } from '@/features/diagnostics/types/diagnostics'

interface DiagnosticsState {
  diagnostics: Record<string, unknown[]>
  diagnosticsByFile: Map<string, Diagnostic[]>
  totalErrors: number
  totalWarnings: number
  actions: {
    setDiagnostics: (filePath: string, diagnostics: Diagnostic[], source?: string) => void
    clearDiagnostics: (filePath: string) => void
  }
}

const useDiagnosticsStoreBase = create<DiagnosticsState>(() => ({
  diagnostics: {},
  diagnosticsByFile: new Map<string, Diagnostic[]>(),
  totalErrors: 0,
  totalWarnings: 0,
  actions: {
    setDiagnostics: (_filePath, _diagnostics, _source) => {},
    clearDiagnostics: (_filePath) => {},
  },
}))

export const useDiagnosticsStore = createSelectors(useDiagnosticsStoreBase)

export function convertLSPDiagnostic(_diagnostic: unknown): unknown { return _diagnostic }
export function convertLintDiagnostic(_filePath: string, _diagnostic: unknown): Diagnostic {
  return _diagnostic as Diagnostic
}
