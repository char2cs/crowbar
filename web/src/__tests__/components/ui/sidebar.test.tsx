import { render } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { SidebarHeader, SidebarFooter } from '@/components/ui/sidebar'

describe('SidebarHeader', () => {
  it('keeps backdrop-blur-sm for the frosted glass effect', () => {
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('backdrop-blur-sm')
  })

  it('does not set its own background (inherits from body)', () => {
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    expect(el.className).not.toContain('bg-chrome-bg')
  })
})

describe('SidebarFooter', () => {
  it('does not set its own background in default mode (inherits from body)', () => {
    const { container } = render(<SidebarFooter>test</SidebarFooter>)
    const el = container.firstChild as HTMLElement
    expect(el.className).not.toContain('bg-chrome-bg')
  })

  it('does not include bg-primary-bg in default mode', () => {
    const { container } = render(<SidebarFooter>test</SidebarFooter>)
    const el = container.firstChild as HTMLElement
    expect(el.className).not.toContain('bg-primary-bg')
  })
})
