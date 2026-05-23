import { render, screen } from '@testing-library/react'
import { AppShell } from '@/components/layout/AppShell'

const sidebar = <div data-testid="sidebar">sidebar</div>
const main = <div data-testid="main">main</div>

test('renders sidebar and main content', () => {
  render(<AppShell sidebar={sidebar}>{main}</AppShell>)
  expect(screen.getByTestId('sidebar')).toBeInTheDocument()
  expect(screen.getByTestId('main')).toBeInTheDocument()
})

test('applies default sidebar width', () => {
  localStorage.clear()
  const { container } = render(<AppShell sidebar={sidebar}>{main}</AppShell>)
  const sidebarEl = container.firstChild?.firstChild as HTMLElement
  expect(sidebarEl.style.width).toBe('256px')
})
