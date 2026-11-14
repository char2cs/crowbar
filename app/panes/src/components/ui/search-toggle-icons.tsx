import {
  TextAa as CaseSensitive,
  BracketsCurly as Regex,
  TextT as WholeWord,
} from '@phosphor-icons/react'

// Icon set for the search option toggles (case / whole-word / regex / preserve-case).
// Kept out of the Search component file so it stays Fast-Refresh-safe.
export const SEARCH_TOGGLE_ICONS = {
  caseSensitive: <CaseSensitive />,
  wholeWord: <WholeWord />,
  regex: <Regex />,
  preserveCase: <span className="ui-font ui-text-xs font-semibold">Aa</span>,
}
