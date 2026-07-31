import {
  TextAa as CaseSensitive,
  BracketsCurly as Regex,
  TextT as WholeWord,
} from '@phosphor-icons/react'

// Icon set for the search option toggles (case / whole-word / regex / preserve-case).
// Kept out of the Search component file so it stays Fast-Refresh-safe.
//
// All four carry the SAME `data-oracle-id`, deliberately: they are four
// renderings of one primitive, exactly as the four live `<Kbd>`s are, so a
// snapshot is one anchor rooted at the glyph itself and `extractSnapshot`'s
// `index` picks which of the four. Four distinct ids would claim four anchors
// no single root contains — `native/oracle/ANCHORS.md` v1.8.
//
// Only `preserveCase` declares anything, because it is the one entry that is a
// text run: a `<span>` blockified by the toggle button's flex, so its width is
// the run's max-content width (v1.5) and its height *is* the run's line box
// (v1.6 — `ui-text-xs` sets a font-size and no line-height, so the box takes
// the button's inherited `sm:text-sm` ratio and floors to it). The three
// phosphor glyphs declare neither: their box is authored by the button's
// `sm:[&_svg:not([class*='size-'])]:size-4`, and they paint no text, so the
// contract can compare their box and nothing else. See
// `native/mapping/search-toggle-icons.md`.
export const SEARCH_TOGGLE_ICONS = {
  caseSensitive: <CaseSensitive data-oracle-id="search-toggle-icon" />,
  wholeWord: <WholeWord data-oracle-id="search-toggle-icon" />,
  regex: <Regex data-oracle-id="search-toggle-icon" />,
  preserveCase: (
    <span
      className="ui-font ui-text-xs font-semibold"
      data-oracle-content-sized="true"
      data-oracle-id="search-toggle-icon"
      data-oracle-line-sized="true"
    >
      Aa
    </span>
  ),
}
