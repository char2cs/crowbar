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
    // `pl-3 pr-3` (12px each) is the container's own `px-1.5` (6px, the
    // sidebar row's `mx-1.5` gutter) plus a row's own `px-1.5`/
    // FILE_TREE_BASE_INDENT (6px, the sidebar row's own `px-1.5` content
    // padding) — file-explorer-tree.tsx and file-explorer-tree-item.tsx.
    const { container } = render(<SidebarHeader>test</SidebarHeader>)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('pl-3')
    expect(el.className).toContain('pr-3')
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
