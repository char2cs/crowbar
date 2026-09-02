import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AffordanceRow } from '@/components/sidebar/affordance-row'

describe('AffordanceRow', () => {
  it('shows only the bubble icon when the parent is not git-capable', () => {
    render(<AffordanceRow onCreateThread={vi.fn()} />)
    expect(screen.getAllByRole('button')).toHaveLength(1)
    expect(screen.queryByTestId('affordance-workspace')).not.toBeInTheDocument()
  })

  it('renders the single-icon trigger visible at rest, not hidden until hover', () => {
    render(<AffordanceRow onCreateThread={vi.fn()} />)
    const trigger = screen.getByRole('button')
    expect(trigger).not.toHaveClass('hidden')
    expect(getComputedStyle(trigger).display).not.toBe('none')
  })

  // Spec §3.5, as read by the fix: "a split control ... with a small dropdown
  // where both are legal" means the row's create surface (two icons) is the
  // disambiguator, not a literal menu widget — so when both actions are
  // legal, both icons render directly, visible at rest, with no intermediate
  // dropdown chrome.
  it('shows both icon buttons, visible at rest, when a workspace is also legal', () => {
    render(<AffordanceRow onCreateThread={vi.fn()} onCreateWorkspace={vi.fn()} />)
    const thread = screen.getByTestId('affordance-thread')
    const workspace = screen.getByTestId('affordance-workspace')
    expect(thread).not.toHaveClass('hidden')
    expect(getComputedStyle(thread).display).not.toBe('none')
    expect(workspace).not.toHaveClass('hidden')
    expect(getComputedStyle(workspace).display).not.toBe('none')
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  })

  it('calls onCreateThread when clicked without onCreateWorkspace', async () => {
    const user = userEvent.setup()
    const onCreateThread = vi.fn()
    render(<AffordanceRow onCreateThread={onCreateThread} />)
    await user.click(screen.getByRole('button'))
    expect(onCreateThread).toHaveBeenCalledOnce()
  })

  it('clicking the bubble icon calls onCreateThread directly, no menu in between', async () => {
    const user = userEvent.setup()
    const onCreateThread = vi.fn()
    const onCreateWorkspace = vi.fn()
    render(<AffordanceRow onCreateThread={onCreateThread} onCreateWorkspace={onCreateWorkspace} />)
    await user.click(screen.getByTestId('affordance-thread'))
    expect(onCreateThread).toHaveBeenCalledOnce()
    expect(onCreateWorkspace).not.toHaveBeenCalled()
  })

  it('clicking the git-mark icon calls onCreateWorkspace directly, no menu in between', async () => {
    const user = userEvent.setup()
    const onCreateThread = vi.fn()
    const onCreateWorkspace = vi.fn()
    render(<AffordanceRow onCreateThread={onCreateThread} onCreateWorkspace={onCreateWorkspace} />)
    await user.click(screen.getByTestId('affordance-workspace'))
    expect(onCreateWorkspace).toHaveBeenCalledOnce()
    expect(onCreateThread).not.toHaveBeenCalled()
  })
})
