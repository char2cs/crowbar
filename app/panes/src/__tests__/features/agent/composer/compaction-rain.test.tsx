import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { CompactionRain } from '@/features/agent/composer/compaction-rain'

describe('CompactionRain', () => {
  it('is decorative, with two falling pattern layers', () => {
    const { container } = render(<CompactionRain width={720} />)

    const svg = container.querySelector('svg')!
    expect(svg).toHaveAttribute('aria-hidden', 'true')
    expect(svg.querySelectorAll('.fall')).toHaveLength(2)
    expect(svg.querySelectorAll('pattern')).toHaveLength(2)
  })

  it('draws faint streaks with the head brightest', () => {
    const { container } = render(<CompactionRain width={330} />)

    const opacities = [...container.querySelectorAll('circle')].map((c) =>
      Number(c.getAttribute('opacity')),
    )
    expect(opacities.length).toBeGreaterThan(20)
    expect(Math.min(...opacities)).toBeGreaterThanOrEqual(0.07)
    expect(Math.max(...opacities)).toBe(0.34)
  })

  it('lays out the same streaks every time for a width, and fewer for the phone', () => {
    const a = render(<CompactionRain width={720} />).container
    const b = render(<CompactionRain width={720} />).container
    const phone = render(<CompactionRain width={330} />).container

    expect(a.innerHTML).toBe(b.innerHTML)
    expect(phone.querySelectorAll('circle').length).toBeLessThan(
      a.querySelectorAll('circle').length,
    )
  })
})
