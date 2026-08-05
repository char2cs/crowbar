import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CollapseSection } from '@/components/layout/collapse-section'

const SECTION = '[data-collapse-section]'
const box = () => document.querySelector<HTMLElement>(SECTION)

describe('what the box renders', () => {
  it('is absent entirely while closed, so a folded section leaves no group behind', () => {
    render(
      <CollapseSection open={false} role="group">
        <div>child</div>
      </CollapseSection>,
    )
    expect(box()).toBeNull()
    expect(screen.queryByText('child')).toBeNull()
  })

  it('carries the role it was given, so the tree still announces depth', () => {
    render(
      <CollapseSection open role="group">
        <div>child</div>
      </CollapseSection>,
    )
    expect(screen.getByRole('group')).toBeInTheDocument()
  })
  it('drops children in the same render that closes the section', () => {
    const { rerender } = render(
      <CollapseSection open>
        <div>child</div>
      </CollapseSection>,
    )

    rerender(
      <CollapseSection open={false}>
        <div>child</div>
      </CollapseSection>,
    )
    expect(screen.queryByText('child')).toBeNull()
  })

  it('never installs a layout-triggering inline transition', () => {
    render(<CollapseSection open>child</CollapseSection>)
    expect(box()!.style.height).toBe('')
    expect(box()!.style.transition).toBe('')
    expect(box()!.style.overflow).toBe('')
  })
})
