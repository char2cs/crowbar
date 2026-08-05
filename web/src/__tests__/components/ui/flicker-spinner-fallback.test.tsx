import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'

// An asset the strip baker refuses (interpolated animation, an animated
// attribute other than fill-opacity, mixed periods — see flicker-strip.ts) must
// still produce a working spinner by inlining the source and letting its own
// SMIL run. Slower, never wrong. This lives in its own file because the
// component memoises baked strips per asset in module state, so the two
// branches cannot be exercised from one module registry.
vi.mock('@/components/ui/flicker-strip', () => ({ buildFlickerStrip: () => null }))

describe('FlickerSpinner when the asset cannot be baked', () => {
  it('falls back to inlining the SMIL asset', () => {
    const { container, getByRole } = render(<FlickerSpinner />)

    expect(getByRole('status', { name: 'Loading' })).toBeInTheDocument()
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    // The unbaked asset, animating itself.
    expect(svg!.querySelector('animate')).not.toBeNull()
    expect(svg!.querySelector('[fill="currentColor"]')).not.toBeNull()
  })

  it('still exposes the marker and the reduced-motion exemption', () => {
    const { getByRole } = render(<FlickerSpinner />)
    expect(getByRole('status')).toHaveAttribute('data-flicker-spinner')
    expect(getByRole('status')).toHaveAttribute('data-essential-motion')
  })
})
