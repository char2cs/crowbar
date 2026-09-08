import { render } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { SidebarHeader, SidebarFooter } from '@/components/ui/sidebar'

describe('SidebarHeader', () => {
  it('keeps backdrop-blur-sm for the frosted glass effect', () => {
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('backdrop-blur-sm')
  })

  it('has no vertical padding — sits flush against its neighbours like any other row', () => {
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    const classes = el.className.split(' ')
    expect(classes).not.toContain('pt-1')
    expect(classes).not.toContain('pb-2')
    expect(classes).not.toContain('py-1')
    expect(classes).not.toContain('p-2')
  })

  it("matches a depth-0 tree row's own horizontal inset exactly", () => {
    // `pl-2.5` (10px) is FILE_TREE_BASE_INDENT (file-explorer-tree-item.tsx);
    // `pr-1.5` (6px) is a row's own untouched `px-1.5` (file-tree-density.ts)
    // — a row's inline `paddingLeft` override only ever touches the left
    // side, so the two sides genuinely differ by design, not by accident.
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('pl-2.5')
    expect(el.className).toContain('pr-1.5')
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
