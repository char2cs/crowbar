import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

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
    expect(css).toContain(
      '*:not(.animate-spin, [data-essential-motion], [data-essential-motion] *)',
    )
  })
})

describe('spinner exemption hooks in the exempted components', () => {
  it('the shared Spinner spins via .animate-spin (covered by the class exemption)', () => {
    expect(read('components/ui/spinner.tsx')).toContain('animate-spin')
  })

  it('toast loading icons (variant-class spin) carry data-essential-motion', () => {
    const toast = read('components/ui/toast.tsx')
    const overlay = read('components/layout/sidebar-toast-overlay.tsx')

    expect(toast.match(/data-essential-motion/g)?.length).toBe(1)
    expect(overlay.match(/data-essential-motion/g)?.length).toBe(1)
    expect(toast).toContain('in-data-[type=loading]:animate-spin')
    expect(overlay).toContain('in-data-[type=loading]:animate-spin')
  })

  it('the flip-dot FlickerSpinner carries data-essential-motion', () => {
    const spinner = read('components/ui/flicker-spinner.tsx')
    expect(spinner).toContain('data-essential-motion')
    expect(spinner).toContain('flicker-strip ${')
  })
})
