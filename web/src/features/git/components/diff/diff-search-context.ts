import { createContext, useContext } from 'react'
import type { DiffSearchMatch } from '../../utils/diff-search'

export interface DiffSearchContextValue {
  /** All matches grouped by file key, for the editors to highlight. */
  matchesByFile: Map<string, DiffSearchMatch[]>
  /** The currently selected match (highlighted distinctly + revealed). */
  active: DiffSearchMatch | null
  /** Bumps on navigation so the owning file's editor re-reveals the active match. */
  revealNonce: number
}

const DiffSearchContext = createContext<DiffSearchContextValue | null>(null)

export const DiffSearchProvider = DiffSearchContext.Provider

/** Read the diff-search layer from within a file section. Null when search is off. */
export function useDiffSearchContext(): DiffSearchContextValue | null {
  return useContext(DiffSearchContext)
}
