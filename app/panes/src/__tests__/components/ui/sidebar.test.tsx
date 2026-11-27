import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, it, expect, vi } from 'vitest'
import { SidebarHeader, SidebarProvider, useSidebar } from '@/components/ui/sidebar'

const realMatchMedia = window.matchMedia
afterEach(() => {
  window.matchMedia = realMatchMedia
})

function Toggle() {
  const { toggleSidebar } = useSidebar()
  return <button onClick={toggleSidebar}>Hide sidebar</button>
}

describe('SidebarProvider', () => {
  it('toggles the one open state at every window width', () => {
    window.matchMedia = ((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    })) as typeof window.matchMedia
    const onOpenChange = vi.fn()
    render(
      <SidebarProvider open onOpenChange={onOpenChange}>
        <Toggle />
      </SidebarProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Hide sidebar' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})

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
