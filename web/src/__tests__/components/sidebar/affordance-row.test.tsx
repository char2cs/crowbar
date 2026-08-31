import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AffordanceRow } from '@/components/sidebar/affordance-row'

describe('AffordanceRow', () => {
  it('shows only the bubble icon when the parent is not git-capable', () => {
    render(<AffordanceRow onCreateThread={vi.fn()} />)
    expect(screen.queryByTestId('affordance-dropdown')).not.toBeInTheDocument()
  })

  it('shows a split-control dropdown when a workspace is also legal', () => {
    render(<AffordanceRow onCreateThread={vi.fn()} onCreateWorkspace={vi.fn()} />)
    expect(screen.getByTestId('affordance-dropdown')).toBeInTheDocument()
  })

  it('calls onCreateThread when clicked without onCreateWorkspace', async () => {
    const user = userEvent.setup()
    const onCreateThread = vi.fn()
    render(<AffordanceRow onCreateThread={onCreateThread} />)
    await user.click(screen.getByRole('button'))
    expect(onCreateThread).toHaveBeenCalledOnce()
  })

  it('opens dropdown menu with Create thread and Create workspace options', async () => {
    const user = userEvent.setup()
    const onCreateThread = vi.fn()
    const onCreateWorkspace = vi.fn()
    render(<AffordanceRow onCreateThread={onCreateThread} onCreateWorkspace={onCreateWorkspace} />)
    await user.click(screen.getByTestId('affordance-dropdown'))
    expect(await screen.findByText('Create thread')).toBeInTheDocument()
    expect(await screen.findByText('Create workspace')).toBeInTheDocument()
  })

  it('calls onCreateThread when the dropdown menu item is clicked', async () => {
    const user = userEvent.setup()
    const onCreateThread = vi.fn()
    const onCreateWorkspace = vi.fn()
    render(<AffordanceRow onCreateThread={onCreateThread} onCreateWorkspace={onCreateWorkspace} />)
    await user.click(screen.getByTestId('affordance-dropdown'))
    await user.click(await screen.findByText('Create thread'))
    expect(onCreateThread).toHaveBeenCalledOnce()
  })

  it('calls onCreateWorkspace when the dropdown menu item is clicked', async () => {
    const user = userEvent.setup()
    const onCreateThread = vi.fn()
    const onCreateWorkspace = vi.fn()
    render(<AffordanceRow onCreateThread={onCreateThread} onCreateWorkspace={onCreateWorkspace} />)
    await user.click(screen.getByTestId('affordance-dropdown'))
    await user.click(await screen.findByText('Create workspace'))
    expect(onCreateWorkspace).toHaveBeenCalledOnce()
  })

  it('does not nest a <button> inside the dropdown trigger (regression: Base UI asChild bug)', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(<AffordanceRow onCreateThread={vi.fn()} onCreateWorkspace={vi.fn()} />)

    const trigger = screen.getByTestId('affordance-dropdown')
    expect(trigger.tagName).toBe('BUTTON')
    expect(trigger.querySelector('button')).toBeNull()

    const errorText = consoleError.mock.calls.map((call) => call.join(' ')).join('\n')
    expect(errorText).not.toMatch(/cannot contain a nested/i)
    expect(errorText).not.toMatch(/asChild/)

    consoleError.mockRestore()
  })
})
