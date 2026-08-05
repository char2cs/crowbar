/**
 * Guards the prefers-reduced-motion kill-switch in src/index.css and its
 * loading-spinner exemptions.
 *
 * The kill-switch zeroes animation/transition durations app-wide under
 * prefers-reduced-motion (WCAG 2.3.3) — but an indeterminate spinner is
 * STATUS, not decorative motion: freezing it turns "still working" into
 * "looks hung". Two exemption hooks must therefore survive refactors:
 *
 *   - `.animate-spin` (Tailwind's spin utility) — covers the shared
 *     <Spinner>, the inline git button spinners and the OOBE checks;
 *   - `[data-essential-motion]` (self or descendants) — covers spinners
 *     animated via a Tailwind VARIANT class such as the toast icons'
 *     `in-data-[type=loading]:animate-spin`, whose generated (escaped)
 *     class name `.animate-spin` does not match.
 *
 * These are source-level assertions on purpose: the exemption is a CSS
 * selector contract between index.css and the exempted sites, and jsdom
 * neither applies media queries nor runs animations, so rendering can't
 * observe it.
 */
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// Not `new URL(rel, import.meta.url)`: Vite statically rewrites that pattern
// into an asset-glob lookup, which yields undefined for .tsx source files.
const HERE = dirname(fileURLToPath(import.meta.url))
const read = (relativeToSrc: string) => readFileSync(resolve(HERE, '..', relativeToSrc), 'utf-8')

describe('prefers-reduced-motion kill-switch (index.css)', () => {
  const css = read('index.css')

  it('has the reduced-motion media query with the duration kill-switch', () => {
    expect(css).toContain('@media (prefers-reduced-motion: reduce)')
    expect(css).toContain('animation-duration: 0.01ms !important')
    expect(css).toContain('transition-duration: 0.01ms !important')
  })

  it('exempts .animate-spin and [data-essential-motion] (self + descendants) from the kill-switch', () => {
    // The exact selector the kill-switch applies to — if someone reverts it
    // to a bare `*`, loading spinners freeze into static icons under
    // prefers-reduced-motion and this fails.
    expect(css).toContain(
      '*:not(.animate-spin, [data-essential-motion], [data-essential-motion] *)',
    )
  })
})

describe('spinner exemption hooks in the exempted components', () => {
  it('the shared Spinner spins via .animate-spin (covered by the class exemption)', () => {
    expect(read('components/ui/spinner.tsx')).toContain('animate-spin')
  })

  it('inline spinners that rely on the .animate-spin exemption still use the plain class', () => {
    for (const file of [
      'features/git/components/branch-section.tsx',
      'components/oobe/oobe-screen.tsx',
    ]) {
      // The plain utility (not only a variant form) must be present — the
      // class exemption matches `.animate-spin` and nothing else.
      expect(read(file), file).toMatch(/[\s"']animate-spin[\s"']/)
    }
  })

  it('toast loading icons (variant-class spin) carry data-essential-motion', () => {
    const toast = read('components/ui/toast.tsx')
    const overlay = read('components/layout/sidebar-toast-overlay.tsx')

    // toast.tsx's anchored icon render and the sidebar overlay's one — each
    // wrapper carries the opt-out attribute because their spin class
    // `in-data-[type=loading]:animate-spin` generates an escaped class name
    // the `.animate-spin` exemption cannot match. (The stacked global viewport
    // was removed — SidebarToastOverlay is the sole viewport for toastManager.)
    expect(toast.match(/data-essential-motion/g)?.length).toBe(1)
    expect(overlay.match(/data-essential-motion/g)?.length).toBe(1)
    expect(toast).toContain('in-data-[type=loading]:animate-spin')
    expect(overlay).toContain('in-data-[type=loading]:animate-spin')
  })

  it('the flip-dot FlickerSpinner carries data-essential-motion', () => {
    // A REGRESSION HAZARD, not a nicety. These spinners used to animate with
    // SMIL, which the kill-switch cannot touch — `animation-duration` says
    // nothing about `<animate>`. Playback is now a CSS animation on a sprite
    // strip, so without this opt-out every flip-dot spinner in the app freezes
    // on frame 0 under prefers-reduced-motion, and the workspace rows, chat
    // glyphs and context pill all report "hung" while work is in flight.
    const spinner = read('components/ui/flicker-spinner.tsx')
    expect(spinner).toContain('data-essential-motion')
    expect(spinner).toContain('flicker-strip ${')
  })
})
