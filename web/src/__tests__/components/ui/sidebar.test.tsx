import { render } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { SidebarHeader, SidebarFooter } from '@/components/ui/sidebar'

describe('SidebarHeader', () => {
  it('uses bg-chrome-bg for its background', () => {
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('bg-chrome-bg')
  })

  it('keeps backdrop-blur-sm for the frosted glass effect', () => {
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('backdrop-blur-sm')
  })
})

describe('SidebarFooter', () => {
  it('uses bg-chrome-bg for its background in default (non-surface) mode', () => {
    const { container } = render(<SidebarFooter>test</SidebarFooter>)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('bg-chrome-bg')
  })

  it('does not include bg-primary-bg in default mode', () => {
    const { container } = render(<SidebarFooter>test</SidebarFooter>)
    const el = container.firstChild as HTMLElement
    expect(el.className).not.toContain('bg-primary-bg')
  })
})
