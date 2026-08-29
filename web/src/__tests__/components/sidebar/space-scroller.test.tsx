import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SpaceScroller } from '@/components/sidebar/space-scroller'
import type { Project } from '@/lib/types'

function makeProject(id: string): Project {
  return {
    id,
    name: id,
    path: `/repos/${id}`,
    lastActivity: new Date('2026-08-28T00:00:00Z'),
  }
}

describe('SpaceScroller', () => {
  beforeEach(() => {
    // jsdom does not implement scrollTo
    HTMLElement.prototype.scrollTo = vi.fn()
  })

  it('renders one panel per project, min-width 100%', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => []}
      />,
    )
    const panels = screen.getAllByTestId('space-panel')
    expect(panels).toHaveLength(2)
    expect(panels[0]).toHaveClass('min-w-full')
  })

  it('clicking a mark scrolls to that space', () => {
    const onChange = vi.fn()
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={onChange}
        rowsForProject={() => []}
      />,
    )
    const el = screen.getByTestId('space-scroll-region')
    fireEvent.wheel(el, { deltaX: 100 })
    Object.defineProperty(el, 'clientWidth', { value: 400, configurable: true })
    Object.defineProperty(el, 'scrollLeft', { value: 400, configurable: true })
    fireEvent.scroll(el)
    expect(onChange).toHaveBeenCalled()
  })
})
