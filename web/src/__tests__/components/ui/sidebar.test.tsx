import { render } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { SidebarHeader, SidebarFooter } from '@/components/ui/sidebar'

describe('SidebarHeader', () => {
  it('keeps backdrop-blur-sm for the frosted glass effect', () => {
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('backdrop-blur-sm')
  })

  it('owns the gap between the sidebar tab switcher and the panel under it', () => {
    // Half of a 10px rhythm the tab bar owns the other half of: the bar is
    // `py-1.5` (6px) and this is `pt-1` (4px), which makes switcher→panel equal
    // the pill→switcher gap above it (pill wrapper `pb-1` + bar `pt-1.5`).
    // It was the symmetric `p-2`, which read 14px here against 6px in the Chats
    // panel — the switcher sat visibly closer to one neighbour than the other.
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('pt-1')
    expect(el.className).toContain('pb-2')
    expect(el.className).toContain('px-2')
    // Not the four-sided shorthand: the top is deliberately not the others.
    expect(el.className.split(' ')).not.toContain('p-2')
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
