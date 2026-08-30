const STORAGE_KEY = 'sidebar-card-height-fraction'

/**
 * CSS custom property the card writes its live height onto (spec §6: "the
 * tree keeps a bottom inset the height of the card"), set on the shared rail
 * ancestor both components sit under in `ide-shell.tsx`. Consumed by
 * `space-scroller.tsx`'s `SpacePanel` via CSS inheritance — a plain `var()`
 * read costs no React re-render, which matters because during a drag this
 * property is written imperatively, once per animation frame, never through
 * React state (see sidebar-carousel.tsx's `applyLiveHeight`). One literal,
 * shared here, so the writer and the reader can never drift apart.
 */
export const CARD_BOTTOM_INSET_VAR = '--card-bottom-inset'

/** Spec §6: the card "opens at one third of the sidebar's height." */
export const DEFAULT_CARD_HEIGHT_FRACTION = 1 / 3
// Keeps a drag from collapsing the card to nothing or swallowing the whole
// tree — the spec states the open default but not these bounds, so they are
// a plain safety clamp, not a design number.
export const MIN_CARD_HEIGHT_FRACTION = 0.15
export const MAX_CARD_HEIGHT_FRACTION = 0.85

export function clampCardHeightFraction(fraction: number): number {
  return Math.min(MAX_CARD_HEIGHT_FRACTION, Math.max(MIN_CARD_HEIGHT_FRACTION, fraction))
}

/**
 * Stored as a proportion of the sidebar rail, not a pixel value — same
 * "remember the user's own commit, not window pressure" idea as
 * `use-sidebar-panel.ts`'s `preferredWidth`, one level down: a fraction
 * survives a window resize without keeping the card a fixed size while
 * everything around it changes.
 */
export function loadCardHeightFraction(): number {
  try {
    const stored = parseFloat(localStorage.getItem(STORAGE_KEY) ?? '')
    return Number.isFinite(stored) ? clampCardHeightFraction(stored) : DEFAULT_CARD_HEIGHT_FRACTION
  } catch {
    return DEFAULT_CARD_HEIGHT_FRACTION
  }
}

export function saveCardHeightFraction(fraction: number): void {
  try {
    localStorage.setItem(STORAGE_KEY, String(clampCardHeightFraction(fraction)))
  } catch {
    // storage unavailable
  }
}
