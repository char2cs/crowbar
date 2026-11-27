import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'

vi.mock('@/components/ui/flicker-strip', () => ({ buildFlickerStrip: () => null }))

describe('FlickerSpinner when the asset cannot be baked', () => {
  it('falls back to inlining the SMIL asset', () => {
    const { container, getByRole } = render(<FlickerSpinner />)

    expect(getByRole('status', { name: 'Loading' })).toBeInTheDocument()
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg!.querySelector('animate')).not.toBeNull()
    expect(svg!.querySelector('[fill="currentColor"]')).not.toBeNull()
  })

  it('still exposes the marker and the reduced-motion exemption', () => {
    const { getByRole } = render(<FlickerSpinner />)
    expect(getByRole('status')).toHaveAttribute('data-flicker-spinner')
    expect(getByRole('status')).toHaveAttribute('data-essential-motion')
  })
})
