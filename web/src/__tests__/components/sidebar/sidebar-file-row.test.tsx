import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  SIDEBAR_FILE_ROW_HEIGHT,
  SidebarFileRow,
  SidebarFileRowCounts,
  TreeGuides,
} from '@/components/sidebar/sidebar-file-row'

describe('SidebarFileRow', () => {
  it('is a 26px button carrying icon, label, dimmed detail, badge and trailing slot', () => {
    render(
      <SidebarFileRow
        icon={<span data-testid="icon" />}
        label="client.ts"
        detail="src/api"
        badge={<span>M</span>}
        trailing={<span>trailing</span>}
      />,
    )
    const row = screen.getByRole('button', { name: /client\.ts/ })
    expect(SIDEBAR_FILE_ROW_HEIGHT).toBe(26)
    expect(row.style.height).toBe('26px')
    expect(row).toContainElement(screen.getByTestId('icon'))
    expect(row).toHaveTextContent('client.tsMsrc/apitrailing')
  })

  it('takes a caller height so a density setting can drive the pitch', () => {
    render(<SidebarFileRow label="a" height={20} />)
    expect(screen.getByRole('button').style.height).toBe('20px')
  })

  it('indents by depth from the base inset', () => {
    render(<SidebarFileRow label="deep" depth={3} indentSize={10} baseIndent={6} />)
    expect(screen.getByRole('button').style.paddingLeft).toBe('36px')
  })

  it('opens on click and on Enter', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<SidebarFileRow label="a.ts" onClick={onClick} />)
    await user.click(screen.getByRole('button'))
    screen.getByRole('button').focus()
    await user.keyboard('{Enter}')
    expect(onClick).toHaveBeenCalledTimes(2)
  })

  it('draws an editor in place of the label as a non-button row', () => {
    render(<SidebarFileRow label="ignored" editor={<input aria-label="name" />} />)
    expect(screen.queryByRole('button')).toBeNull()
    expect(screen.queryByText('ignored')).toBeNull()
    expect(screen.getByLabelText('name')).toBeInTheDocument()
  })

  it('draws one guide per level, carrying its target for delegated handlers', () => {
    const { container } = render(
      <SidebarFileRow
        label="leaf"
        depth={2}
        guides={
          <TreeGuides
            depth={2}
            baseIndent={6}
            indentSize={14}
            previousDepth={2}
            nextDepth={0}
            targets={[{ path: '/r/src', name: 'src', isDir: true, isActive: true }, null]}
          />
        }
      />,
    )
    const guides = container.querySelectorAll('.file-tree-guide')
    expect(guides).toHaveLength(2)
    expect(guides[0].getAttribute('data-file-path')).toBe('/r/src')
    expect(guides[0].getAttribute('title')).toBe('src')
    expect(guides[1].getAttribute('data-file-path')).toBeNull()
  })
})

describe('SidebarFileRowCounts', () => {
  it('formats counts through the shared change-count formatter', () => {
    render(<SidebarFileRowCounts additions={1500} deletions={3} />)
    expect(screen.getByText('+2k')).toBeInTheDocument()
    expect(screen.getByText('-3')).toBeInTheDocument()
  })

  it('omits a zero side and renders nothing when both are zero', () => {
    const { container, rerender } = render(<SidebarFileRowCounts additions={0} deletions={4} />)
    expect(screen.queryByText(/^\+/)).toBeNull()
    expect(screen.getByText('-4')).toBeInTheDocument()
    rerender(<SidebarFileRowCounts additions={0} deletions={0} />)
    expect(container).toBeEmptyDOMElement()
  })
})
